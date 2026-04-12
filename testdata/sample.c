#include <stdio.h>
#include <stdlib.h>
#include "myheader.h"

#define MAX_RETRIES 3
#define DEFAULT_TIMEOUT 30
#define SQUARE(x) ((x) * (x))

typedef int Status;
typedef struct Node Node;
typedef void (*CallbackFunc)(int code, const char *msg);

enum LogLevel {
    LOG_DEBUG,
    LOG_INFO,
    LOG_WARN,
    LOG_ERROR
};

struct Config {
    const char *host;
    int port;
    int debug;
};

struct Server {
    struct Config config;
    int running;
};

static int helper_count = 0;

int global_timeout = 60;

struct Server *server_new(struct Config cfg);

void server_start(struct Server *s);

static void helper(void);

int format_status(Status s) {
    switch (s) {
    case 0: return 0;
    default: return -1;
    }
}

struct Server *server_new(struct Config cfg) {
    struct Server *s = malloc(sizeof(struct Server));
    s->config = cfg;
    s->running = 0;
    return s;
}

void server_start(struct Server *s) {
    s->running = 1;
}

static void helper(void) {
    helper_count++;
}

void server_free(struct Server *s) {
    free(s);
}
