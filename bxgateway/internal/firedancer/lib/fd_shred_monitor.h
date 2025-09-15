#ifndef FD_SHRED_MONITOR_H
#define FD_SHRED_MONITOR_H

#include <stdint.h>
#include <stdbool.h>

#define FD_SHRED_MAX_SZ 1228U

// Simplified shred info struct for Go (avoids CGO packed struct issues)
typedef struct {
    uint8_t* raw_data;    // Pointer to actual shred data (no copy!)
    uint32_t size;
    uint8_t  variant;
    uint64_t slot;
    uint32_t idx;
    uint64_t batch_seq;
} shred_info_t;

#ifdef __cplusplus
extern "C" {
#endif

// Initialize the monitor
// Returns: 0 = success, -1 = error
int shred_monitor_init(const char* workspace_path);

// Get next available shred (non-blocking)
// Returns: 1 = got shred, 0 = no new shreds, -1 = error
int shred_monitor_get_next(shred_info_t* shred_info);

// Stop monitoring
void shred_monitor_stop();

// Check if monitor is running
bool shred_monitor_is_running();

// Cleanup resources
void shred_monitor_cleanup();

#ifdef __cplusplus
}
#endif

#endif // FD_SHRED_MONITOR_H
