package coremain

import (
	"runtime"
	"runtime/debug"
)

// ForceGC 同步执行 GC 并将空闲堆内存归还 OS。
// 用于重载前必须先回收旧 matcher 内存的场景（避免新旧并存峰值）。
func ForceGC() {
	runtime.GC()
	debug.FreeOSMemory()
}
