// Vulkan helper library — skeleton test sample
#include <cstdint>
#include <vector>
#include "vulkan/vulkan.h"
#include "utils.hpp"

#define VK_MAX_FRAMES_IN_FLIGHT 2
#define VK_CHECK(result) checkVkResult(result, __FILE__, __LINE__)
#define ROUND_UP(x, align) (((x) + (align) - 1) & ~((align) - 1))

using BufferHandle = uint64_t;
using ImageHandle = uint64_t;

enum class PipelineType {
    Graphics,
    Compute,
    RayTracing,
};

enum class QueueFamily {
    Graphics = 0,
    Compute  = 1,
    Transfer = 2,
};

struct Vertex {
    float x, y, z;
    float nx, ny, nz;
    float u, v;
};

struct BufferDesc {
    uint64_t size;
    uint32_t usage;
    bool     hostVisible;
};

namespace vk {

class Pipeline {
public:
    Pipeline(VkDevice device, PipelineType type);
    ~Pipeline();

    void bind(VkCommandBuffer cmd);
    VkPipeline handle() const;
    bool isValid() const;

private:
    VkDevice   device_;
    VkPipeline pipeline_;
    PipelineType type_;
};

class CommandBuffer {
public:
    explicit CommandBuffer(VkDevice device, VkCommandPool pool);
    ~CommandBuffer();

    void begin();
    void end();
    void reset();
    VkCommandBuffer raw() const;

private:
    VkDevice        device_;
    VkCommandPool   pool_;
    VkCommandBuffer buffer_;
};

namespace util {

template<typename T>
T alignUp(T value, T alignment) {
    return (value + alignment - 1) & ~(alignment - 1);
}

template<typename T>
bool contains(const std::vector<T>& vec, const T& item);

} // namespace util
} // namespace vk

// free function — top-level
VkResult createInstance(const char* appName, VkInstance* outInstance);
void destroyInstance(VkInstance instance);

// extern variable
extern uint32_t g_frameIndex;

static void internalHelper() {}
