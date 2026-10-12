//go:build !vim4

package native

func NewBackend(opts NativeOptions) (NativeInterface, error) {
	return NewNativeProxy(opts)
}
