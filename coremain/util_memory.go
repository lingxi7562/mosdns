package coremain

import (
	"net/http"
	"runtime"
	"runtime/debug"
	"time"
)

// ManualGC 异步触发一次完整的 GC，并将空闲堆内存归还操作系统。
// 使用延迟执行避免阻塞当前调用链（重载/API 路径），
// 同时确保旧 matcher 内存被及时回收，防止规则更新时内存峰值累积导致 OOM。
// 注意：runtime.GC() 只回收不可达对象，空闲 span 仍在进程 RSS 中；
// debug.FreeOSMemory() 才会真正把空闲内存归还 OS（设备无 swap，RSS 直接反映内存压力）。
func ManualGC() {
	time.AfterFunc(3*time.Second, func() {
		runtime.GC()
		debug.FreeOSMemory()
	})
}

// WithAsyncGC 保持原有语义：注册一个在请求完成后异步执行 GC 的包装器。
func WithAsyncGC(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
		go ManualGC()
	}
}
