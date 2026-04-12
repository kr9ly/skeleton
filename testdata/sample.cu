#include <stdio.h>
#include <cuda_runtime.h>

#define BLOCK_SIZE 256
#define GRID_SIZE(n) (((n) + BLOCK_SIZE - 1) / BLOCK_SIZE)

typedef struct {
    float *data;
    int size;
} Vector;

__global__ void vector_add(const float *a, const float *b, float *c, int n) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx < n) {
        c[idx] = a[idx] + b[idx];
    }
}

__global__ void vector_scale(float *v, float scalar, int n) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx < n) {
        v[idx] *= scalar;
    }
}

__device__ float device_helper(float x, float y) {
    return x * y + y;
}

__host__ __device__ float host_device_func(float x) {
    return x * x;
}

static __device__ float internal_helper(float x) {
    return x + 1.0f;
}

__constant__ float scale_factor;

__shared__ float shared_buffer[256];

void launch_add(const float *a, const float *b, float *c, int n) {
    int grid = GRID_SIZE(n);
    vector_add<<<grid, BLOCK_SIZE>>>(a, b, c, n);
    cudaDeviceSynchronize();
}

enum DeviceType {
    DEVICE_CPU,
    DEVICE_GPU,
    DEVICE_AUTO
};

struct Context {
    enum DeviceType device;
    int device_id;
    cudaStream_t stream;
};
