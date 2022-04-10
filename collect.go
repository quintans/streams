package stream

func ToSlice[T any]() func(t T) []T {
	var a []T
	return func(t T) []T {
		a = append(a, t)
		return a
	}
}

func ToSet[T comparable]() func(t T) map[T]bool {
	m := map[T]bool{}
	return func(t T) map[T]bool {
		m[t] = true
		return m
	}
}
