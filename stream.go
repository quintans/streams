package stream

import (
	"sync"
	"time"
)

type Observer[T any] struct {
	Next     func(data T) bool
	Error    func(e error)
	Complete func()
}

type (
	Subscription       func()
	Stream[T any]      func(observer Observer[T]) Subscription
	Operator[T, U any] func(source Stream[T]) Stream[U]
)

func Periodic(interval time.Duration) Stream[int] {
	return func(observer Observer[int]) Subscription {
		i := 0
		ticker := time.NewTicker(interval)
		var closeCh sync.Once
		done := make(chan struct{})
		unsub := func() {
			closeCh.Do(func() {
				ticker.Stop()
				close(done)
				if observer.Complete != nil {
					observer.Complete()
				}
			})
		}
		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					ok := observer.Next(i)
					if !ok {
						unsub()
					}
					i++
				}
			}
		}()

		return unsub
	}
}

func From[T any](args ...T) Stream[T] {
	return func(observer Observer[T]) Subscription {
		for _, v := range args {
			if !observer.Next(v) {
				break
			}
		}
		if observer.Complete != nil {
			observer.Complete()
		}

		return func() {}
	}
}

// Accept allows us to push values to the stream
func Accept[T any](completed func()) (accept func(T) bool, s Stream[T]) {
	var obs *Observer[T]
	accept = func(t T) bool {
		if obs == nil {
			return false
		}
		return obs.Next(t)
	}
	s = func(observer Observer[T]) Subscription {
		obs = &observer
		return func() {
			obs = nil
			if observer.Complete != nil {
				observer.Complete()
			}
			if completed != nil {
				completed()
			}
		}
	}
	return
}

type Listener[T any] interface {
	Listen(func(T))
	Close()
}

func FromListener[T any](listener Listener[T]) Stream[T] {
	return func(observer Observer[T]) Subscription {
		listener.Listen(func(e T) {
			observer.Next(e)
		})

		return func() {
			listener.Close()
			if observer.Complete != nil {
				observer.Complete()
			}
		}
	}
}

func Subscribe[T any](obs Observer[T]) func(source Stream[T]) Subscription {
	return func(source Stream[T]) Subscription {
		return source(obs)
	}
}

func Observe[T any](f func(T) bool) func(source Stream[T]) Subscription {
	obs := Observer[T]{
		Next: f,
	}
	return func(source Stream[T]) Subscription {
		return source(obs)
	}
}

func ForEach[T any](f func(T)) func(source Stream[T]) Subscription {
	obs := Observer[T]{
		Next: func(t T) bool {
			f(t)
			return true
		},
	}
	return func(source Stream[T]) Subscription {
		return source(obs)
	}
}

func Collect[T any, X any](fn func(T) X) func(source Stream[T]) X {
	var x X
	obs := Observer[T]{
		Next: func(t T) bool {
			x = fn(t)
			return true
		},
	}
	return func(source Stream[T]) X {
		_ = source(obs)
		return x
	}
}

func Map[T, U any](f func(x T) U) Operator[T, U] {
	return func(source Stream[T]) Stream[U] {
		return func(observer Observer[U]) Subscription {
			return source(Observer[T]{
				Next: func(data T) bool {
					return observer.Next(f(data))
				},
				Error:    observer.Error,
				Complete: observer.Complete,
			})
		}
	}
}

func Filter[T any](f func(x T) bool) Operator[T, T] {
	return func(source Stream[T]) Stream[T] {
		return func(observer Observer[T]) Subscription {
			return source(Observer[T]{
				Next: func(data T) bool {
					if f(data) {
						return observer.Next(data)
					}
					return true
				},
				Error:    observer.Error,
				Complete: observer.Complete,
			})
		}
	}
}

func Take[T any](n int) Operator[T, T] {
	return func(source Stream[T]) Stream[T] {
		return func(observer Observer[T]) Subscription {
			i := 0
			return source(Observer[T]{
				Next: func(data T) bool {
					ok := observer.Next(data)
					i++
					if !ok || i == n {
						return false
					}
					return ok
				},
				Error:    observer.Error,
				Complete: observer.Complete,
			})
		}
	}
}

func Merge[T any](streams ...Stream[T]) Stream[T] {
	return func(observer Observer[T]) Subscription {
		unsubscribes := make([]Subscription, len(streams))
		numCompleted := 0

		for i, stream := range streams {
			unsubscribes[i] = stream(Observer[T]{
				Next: observer.Next,
				Complete: func() {
					numCompleted++
					if numCompleted == len(streams) {
						if observer.Complete != nil {
							observer.Complete()
						}
					}
				},
				Error: observer.Error,
			})
		}

		return func() {
			for _, unsub := range unsubscribes {
				unsub()
			}
		}
	}
}

func Reduce[T, U any](initialValue U, f func(acc U, current T) U) Operator[T, U] {
	return func(source Stream[T]) Stream[U] {
		return func(observer Observer[U]) Subscription {
			current := initialValue
			ok := observer.Next(current)
			if !ok {
				return func() {}
			}
			return source(Observer[T]{
				Next: func(data T) bool {
					current = f(current, data)
					return observer.Next(current)
				},
				Error:    observer.Error,
				Complete: observer.Complete,
			})
		}
	}
}

func StartWith[T any](data T) Operator[T, T] {
	return func(source Stream[T]) Stream[T] {
		return func(observer Observer[T]) Subscription {
			ok := observer.Next(data)
			if !ok {
				return func() {}
			}
			return source(observer)
		}
	}
}

func Flatten[T any]() func(Stream[Stream[T]]) Stream[T] {
	return func(outer Stream[Stream[T]]) Stream[T] {
		return func(observer Observer[T]) Subscription {
			var innerSub Subscription
			outerCompleted := false

			unsubscribe := outer(Observer[Stream[T]]{
				Next: func(stream Stream[T]) bool {
					if innerSub != nil {
						innerSub()
					}

					innerSub = stream(Observer[T]{
						Next: func(data T) bool {
							return observer.Next(data)
						},
						Error: observer.Error,
						Complete: func() {
							innerSub = nil
							if outerCompleted && observer.Complete != nil {
								observer.Complete()
							}
						},
					})
					return true
				},
				Error: observer.Error,
				Complete: func() {
					outerCompleted = true
				},
			})

			return func() {
				unsubscribe()
				if innerSub != nil {
					innerSub()
				}
			}
		}
	}
}

func Pipe2[A, B any](a A, fn1 func(A) B) B {
	return fn1(a)
}

func Pipe3[A, B, C any](a A, fn1 func(A) B, fn2 func(B) C) C {
	return fn2(fn1(a))
}

func Pipe4[A, B, C, D any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D) D {
	return fn3(fn2(fn1(a)))
}

func Pipe5[A, B, C, D, E any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D, fn4 func(D) E) E {
	return fn4(fn3(fn2(fn1(a))))
}

func Pipe6[A, B, C, D, E, F any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D, fn4 func(D) E, fn5 func(E) F) F {
	return fn5(fn4(fn3(fn2(fn1(a)))))
}

func Pipe7[A, B, C, D, E, F, G any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D, fn4 func(D) E, fn5 func(E) F, fn6 func(F) G) G {
	return fn6(fn5(fn4(fn3(fn2(fn1(a))))))
}

func Pipe8[A, B, C, D, E, F, G, H any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D, fn4 func(D) E, fn5 func(E) F, fn6 func(F) G, fn7 func(G) H) H {
	return fn7(fn6(fn5(fn4(fn3(fn2(fn1(a)))))))
}

func Pipe9[A, B, C, D, E, F, G, H, I any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D, fn4 func(D) E, fn5 func(E) F, fn6 func(F) G, fn7 func(G) H, fn8 func(H) I) I {
	return fn8(fn7(fn6(fn5(fn4(fn3(fn2(fn1(a))))))))
}

func Pipe10[A, B, C, D, E, F, G, H, I, J any](a A, fn1 func(A) B, fn2 func(B) C, fn3 func(C) D, fn4 func(D) E, fn5 func(E) F, fn6 func(F) G, fn7 func(G) H, fn8 func(H) I, fn9 func(I) J) J {
	return fn9(fn8(fn7(fn6(fn5(fn4(fn3(fn2(fn1(a)))))))))
}
