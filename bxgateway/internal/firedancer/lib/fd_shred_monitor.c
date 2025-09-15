#ifdef __APPLE__
#include <machine/endian.h>
#include <libkern/OSByteOrder.h>
#define le64toh(x) OSSwapLittleToHostInt64(x)
#define htole64(x) OSSwapHostToLittleInt64(x)
#define le32toh(x) OSSwapLittleToHostInt32(x)
#define htole32(x) OSSwapHostToLittleInt32(x)
#define le16toh(x) OSSwapLittleToHostInt16(x)
#define htole16(x) OSSwapHostToLittleInt16(x)
#else
#include <endian.h>
#endif

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <unistd.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <signal.h>
#include <time.h>
#include <errno.h>
#include <stdint.h>
#include <stdbool.h>
#include <inttypes.h>

#include "fd_shred_monitor.h"

// All the same defines and structures
#define FD_WKSP_MAGIC               0xF17EDA2C3731C591ULL
#define FD_MCACHE_MAGIC             0xF17EDA2C373CA540UL
#define FD_DCACHE_MAGIC             0xF17EDA2C37DCA540UL
#define FD_SHRED_STORE_MTU          41792U
#define FD_MCACHE_ALIGN             128U
#define FD_DCACHE_ALIGN             128U
#define FD_CHUNK_LG_SZ              6
#define SHRED34_MAX_COUNT           34U

static const char* DEFAULT_WORKSPACE_PATHS[] = {
    "/mnt/.fd/.gigantic/fd1_shred_store.wksp",
    NULL
};

static const char *FD_LIB_LOG_PREFIX = "[FD_LIB]";

static inline void log_error(const char* fmt, ...) {
    va_list args;
    va_start(args, fmt);
    fprintf(stderr, "%s: ", FD_LIB_LOG_PREFIX);
    vfprintf(stderr, fmt, args);
    va_end(args);
}

static inline void log_info(const char* fmt, ...) {
    va_list args;
    va_start(args, fmt);
    fprintf(stderr, "%s: ", FD_LIB_LOG_PREFIX);
    vfprintf(stderr, fmt, args);
    va_end(args);
}

typedef struct {
    uint64_t magic;
    uint64_t part_max;
    uint64_t data_max;
    uint64_t gaddr_lo;
    uint64_t gaddr_hi;
    char     name[40];
    uint32_t seed;
    uint32_t padding1;
    uint32_t idle_top_cidx;
    uint32_t part_head_cidx;
    uint32_t part_tail_cidx;
    uint32_t part_used_cidx;
    uint32_t part_free_cidx;
    uint64_t cycle_tag;
    uint64_t owner;
    char     padding2[72];
} fd_wksp_private_t;

typedef union __attribute__((aligned(32))) {
    struct {
        uint64_t seq;
        uint64_t sig;
        uint32_t chunk;
        uint16_t sz;
        uint16_t ctl;
        uint32_t tsorig;
        uint32_t tspub;
    };
} fd_frag_meta_t;

typedef struct {
    uint8_t  signature[64];
    uint8_t  variant;
    uint64_t slot;
    uint32_t idx;
    uint16_t version;
    uint32_t fec_set_idx;
    uint8_t  payload[];
} __attribute__((packed)) fd_shred_t;

typedef struct __attribute__((aligned(64))) {
    uint64_t shred_cnt;
    uint64_t est_txn_cnt;
    uint64_t stride;
    uint64_t offset;
    uint64_t shred_sz;
    union {
        fd_shred_t shred;
        uint8_t buffer[FD_SHRED_MAX_SZ];
    } pkts[SHRED34_MAX_COUNT];
} fd_shred34_t;

typedef struct __attribute__((aligned(FD_MCACHE_ALIGN))) {
    uint64_t magic;
    uint64_t depth;
    uint64_t app_sz;
    uint64_t seq0;
    uint64_t app_off;
    uint64_t __attribute__((aligned(FD_MCACHE_ALIGN))) seq[16];
} fd_mcache_private_hdr_t;

typedef struct __attribute__((aligned(FD_DCACHE_ALIGN))) {
    uint64_t magic;
    uint64_t data_sz;
    uint64_t app_sz;
    uint64_t app_off;
    uint8_t __attribute__((aligned(FD_DCACHE_ALIGN))) guard[FD_DCACHE_ALIGN];
} fd_dcache_private_hdr_t;

// Monitor context
typedef struct {
    const char* workspace_path;
    int         workspace_fd;
    void*       workspace_mapping;
    size_t      workspace_size;

    fd_wksp_private_t* wksp_private;
    fd_frag_meta_t*    mcache;
    uint64_t    mcache_depth;


    bool        running;

    // Internal state for iteration
    uint64_t    current_batch_seq;
    uint32_t    current_shred_idx;
    fd_shred34_t* current_batch;
    uint64_t    last_seq;
} shred_monitor_ctx_t;

static shred_monitor_ctx_t* g_monitor_ctx = NULL;

static int find_mcache(shred_monitor_ctx_t* ctx) {
    uint8_t* workspace_base = (uint8_t*)ctx->workspace_mapping;
    size_t workspace_size = ctx->workspace_size;

    for (size_t offset = 128; offset < workspace_size - sizeof(fd_mcache_private_hdr_t); offset += 128) {
        fd_mcache_private_hdr_t* hdr = (fd_mcache_private_hdr_t*)(workspace_base + offset);

        if (hdr->magic == FD_MCACHE_MAGIC) {
            ctx->mcache = (fd_frag_meta_t*)(hdr + 1);
            ctx->mcache_depth = hdr->depth;

            for (size_t dc_offset = 0; dc_offset < workspace_size - sizeof(fd_dcache_private_hdr_t); dc_offset += 128) {
                fd_dcache_private_hdr_t* dcache_hdr = (fd_dcache_private_hdr_t*)(workspace_base + dc_offset);
                if (dcache_hdr->magic == FD_DCACHE_MAGIC) {
                    return 0;
                }
            }
            return -1;
        }
    }
    return -1;
}

static int attach_workspace(shred_monitor_ctx_t* ctx) {
    struct stat st;
    if (stat(ctx->workspace_path, &st) != 0) {
        log_error("cannot access workspace: %s\n", strerror(errno));
        return -1;
    }

    ctx->workspace_fd = open(ctx->workspace_path, O_RDONLY);
    if (ctx->workspace_fd < 0) {
        log_error("cannot open workspace: %s\n", strerror(errno));
        return -1;
    }

    ctx->workspace_size = st.st_size;
    ctx->workspace_mapping = mmap(NULL, ctx->workspace_size, PROT_READ, MAP_SHARED, ctx->workspace_fd, 0);
    if (ctx->workspace_mapping == MAP_FAILED) {
        log_error("cannot mmap workspace: %s\n", strerror(errno));
        close(ctx->workspace_fd);
        return -1;
    }

    ctx->wksp_private = (fd_wksp_private_t*)ctx->workspace_mapping;

    if (ctx->wksp_private->magic != FD_WKSP_MAGIC) {
        log_error("invalid workspace magic: expected 0x%llx, got 0x%llx\n",
            (unsigned long long)FD_WKSP_MAGIC, (unsigned long long)ctx->wksp_private->magic);
        munmap(ctx->workspace_mapping, ctx->workspace_size);
        close(ctx->workspace_fd);
        return -1;
    }

    log_info("workspace attached: %s (%.2f MB)\n",
           ctx->wksp_private->name, (double)ctx->workspace_size / (1024.0 * 1024.0));

    return find_mcache(ctx);
}

static fd_shred34_t* load_batch_from_chunk(shred_monitor_ctx_t* ctx, uint32_t chunk) {
    // Chunks are file offsets, workspace is memory-mapped
    uint64_t file_offset = (uint64_t)chunk << FD_CHUNK_LG_SZ;

    // Bounds check against workspace size
    if (file_offset + sizeof(fd_shred34_t) > ctx->workspace_size) {
        log_error("batch offset out of bounds: %llu > %zu\n", (unsigned long long)(file_offset + sizeof(fd_shred34_t)), ctx->workspace_size);
        return NULL;
    }

    // Direct memory access to memory-mapped workspace
    uint8_t* workspace_base = (uint8_t*)ctx->workspace_mapping;
    fd_shred34_t* batch = (fd_shred34_t*)(workspace_base + file_offset);

    // More lenient validation
    if (batch->shred_cnt == 0) {
        log_error("empty batch at chunk %u\n", chunk);
        return NULL;
    }

    if (batch->shred_cnt > SHRED34_MAX_COUNT) {
        log_error("large batch at chunk %u: %llu shreds\n", chunk, (unsigned long long)batch->shred_cnt);
        // Don't return NULL, just cap the count
        batch->shred_cnt = SHRED34_MAX_COUNT;
    }

    return batch;
}

// ===== PUBLIC API FUNCTIONS =====

// Initialize the monitor
int shred_monitor_init(const char* workspace_path) {
    if (g_monitor_ctx) {
        log_error("monitor already initialized\n");
        return -1;
    }

    g_monitor_ctx = calloc(1, sizeof(shred_monitor_ctx_t));
    if (!g_monitor_ctx) {
        log_error("failed to allocate monitor context\n");
        return -1;
    }

    // Auto-detect workspace if not specified
    if (!workspace_path || strlen(workspace_path) == 0) {
        for (int i = 0; DEFAULT_WORKSPACE_PATHS[i]; i++) {
            if (access(DEFAULT_WORKSPACE_PATHS[i], R_OK) == 0) {
                workspace_path = DEFAULT_WORKSPACE_PATHS[i];
                break;
            }
        }
        if (!workspace_path) {
            log_error("no workspace path specified and no default workspace found\n");
            free(g_monitor_ctx);
            g_monitor_ctx = NULL;
            return -1;
        }
    }

    g_monitor_ctx->workspace_path = workspace_path;
    if (attach_workspace(g_monitor_ctx) != 0) {
        log_error("failed to attach workspace: %s\n", workspace_path);
        free(g_monitor_ctx);
        g_monitor_ctx = NULL;
        return -1;
    }

    // Find the latest sequence number in mcache and start from there
    uint64_t max_seq = 0;
    for (uint64_t i = 0; i < g_monitor_ctx->mcache_depth; i++) {
        fd_frag_meta_t* frag = &g_monitor_ctx->mcache[i];
        if (frag->seq > max_seq)
            max_seq = frag->seq;
    }
    g_monitor_ctx->last_seq = max_seq;
    g_monitor_ctx->running = 1;

    log_info("monitor initialized with latest seq=%llu\n", (unsigned long long)max_seq);
    return 0;
}

// Get next available shred (non-blocking)
// Returns: 1 = got shred, 0 = no new shreds, -1 = error
int shred_monitor_get_next(shred_info_t* shred_info) {
    if (!g_monitor_ctx || !shred_info || !g_monitor_ctx->running) {
        return -1;
    }

    // If we're in the middle of processing a batch, continue with it
    if (g_monitor_ctx->current_batch &&
        g_monitor_ctx->current_shred_idx < g_monitor_ctx->current_batch->shred_cnt) {
        fd_shred34_t* batch = g_monitor_ctx->current_batch;
        uint32_t shred_idx = g_monitor_ctx->current_shred_idx++;

        uint8_t* shred_data = ((uint8_t*)batch) + batch->offset + (shred_idx * batch->stride);
        const fd_shred_t* shred = (const fd_shred_t*)shred_data;

        // Point to shred data (no copy!)
        memset(shred_info, 0, sizeof(shred_info_t));
        shred_info->raw_data = shred_data;
        shred_info->size = FD_SHRED_MAX_SZ;
        shred_info->variant = shred->variant;
        shred_info->slot = shred->slot;
        shred_info->idx = shred->idx;
        shred_info->batch_seq = g_monitor_ctx->current_batch_seq;

        return 1;
    }

    // Calculate next position directly from sequence number
    uint64_t next_seq = g_monitor_ctx->last_seq + 1;
    uint64_t idx = next_seq % g_monitor_ctx->mcache_depth;

    fd_frag_meta_t* frag = &g_monitor_ctx->mcache[idx];
    if (frag->seq == next_seq && frag->sz > 0 && frag->sz <= FD_SHRED_STORE_MTU) {
        // Load the batch
        fd_shred34_t* batch = load_batch_from_chunk(g_monitor_ctx, frag->chunk);
        if (!batch) {
            log_error("failed to load batch from chunk %u\n", frag->chunk);
            return -1;
        }

        g_monitor_ctx->current_batch = batch;
        g_monitor_ctx->current_batch_seq = frag->seq;
        g_monitor_ctx->current_shred_idx = 0;
        g_monitor_ctx->last_seq = next_seq;

        // Process first shred
        return shred_monitor_get_next(shred_info);
    }

    return 0;
}

// Stop monitoring
void shred_monitor_stop() {
    if (g_monitor_ctx) {
        g_monitor_ctx->running = 0;
    }
}

// Check if monitor is running
bool shred_monitor_is_running() {
    return g_monitor_ctx && g_monitor_ctx->running;
}

// Cleanup
void shred_monitor_cleanup() {
    if (g_monitor_ctx) {
        if (g_monitor_ctx->workspace_mapping) {
            munmap(g_monitor_ctx->workspace_mapping, g_monitor_ctx->workspace_size);
        }
        if (g_monitor_ctx->workspace_fd >= 0) {
            close(g_monitor_ctx->workspace_fd);
        }
        free(g_monitor_ctx);
        g_monitor_ctx = NULL;
    }
}
