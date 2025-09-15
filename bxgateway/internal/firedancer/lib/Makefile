CC = gcc
CFLAGS = -Wall -Wextra -O2 -std=c11 -D_GNU_SOURCE -fPIC
AR = ar
ARFLAGS = rcs

# Platform specific flags and targets
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_S),Darwin)
    CFLAGS += -D__APPLE__
    STATIC_LIB = libfdshredmonitor.a
    DYNAMIC_LIB = libfdshredmonitor.dylib
    DYNAMIC_FLAGS = -dynamiclib -install_name @rpath/$(DYNAMIC_LIB)
else
    STATIC_LIB = libfdshredmonitor.a
endif

SOURCES = fd_shred_monitor.c
OBJECTS = $(SOURCES:.c=.o)

.PHONY: all clean linux-amd64 linux-clean

all: $(STATIC_LIB)
ifeq ($(UNAME_S),Darwin)
all: $(DYNAMIC_LIB)
endif

linux-amd64:
	@echo "🔨 Building fd-shred-monitor library for Linux AMD64..."
	@mkdir -p amd64-dist
	docker run --rm --platform=linux/amd64 -v "$(PWD):/src" -v "$(PWD)/amd64-dist:/output" -w /src gcc:latest sh -c 'make clean all && cp fd_shred_monitor.h /output/ && cp libfdshredmonitor.a /output/'
	@echo "✅ Library built successfully!"
	@echo "📦 Output: amd64-dist/libfdshredmonitor.a"
	@echo "📄 Header: amd64-dist/fd_shred_monitor.h"

linux-clean:
	rm -rf amd64-dist

$(STATIC_LIB): $(OBJECTS)
	$(AR) $(ARFLAGS) $@ $^

$(DYNAMIC_LIB): $(OBJECTS)
	$(CC) $(DYNAMIC_FLAGS) -o $@ $^

%.o: %.c
	$(CC) $(CFLAGS) -c $< -o $@

clean: linux-clean
	rm -f $(OBJECTS) $(STATIC_LIB) $(DYNAMIC_LIB)

re: clean all
