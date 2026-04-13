package utils

func AnyPTR[T any](ptr T) *T {
	return &ptr
}
