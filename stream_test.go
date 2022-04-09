package stream_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/quintans/stream"
	"github.com/stretchr/testify/require"
)

func TestOf_CollectSlice(t *testing.T) {
	a := stream.Pipe2(
		stream.Of(1, 2, 3),
		stream.Collect(stream.ToSlice[int]()),
	)
	require.ElementsMatch(t, []int{1, 2, 3}, a)
}

func TestOf_CollectSet(t *testing.T) {
	set := stream.Pipe2(
		stream.Of(1, 2, 3, 2),
		stream.Collect(stream.ToSet[int]()),
	)
	require.EqualValues(t, map[int]bool{1: true, 2: true, 3: true}, set)
}

func TestOf_Reduce_Observe(t *testing.T) {
	val := 0
	stream.Pipe3(
		stream.Of(1, 2, 3),
		stream.Reduce(10, func(acc, i int) int {
			return acc + i
		}),
		stream.Observe(func(i int) bool {
			val = i
			return true
		}),
	)
	require.Equal(t, 16, val)
}

func TestOf_Flatten_ForEach(t *testing.T) {
	cnt := 0
	stream.Pipe4(
		stream.Of(1, 3, 5),
		stream.Map(func(x int) stream.Stream[int] {
			return stream.Of(x)
		}),
		stream.Flatten[int](),
		stream.ForEach(func(i int) {
			cnt++
		}),
	)
	require.Equal(t, 3, cnt)
}

func TestOf_Take_Map_StartWith_ForEach(t *testing.T) {
	cnt := 0
	stream.Pipe5(
		stream.Of(1, 2, 3, 4, 5, 6),
		stream.Take[int](5),
		stream.Map(func(i int) int {
			return i * 2
		}),
		stream.StartWith(10),
		stream.ForEach(func(i int) {
			cnt++
		}),
	)
	require.Equal(t, 6, cnt)
}

func TestPeriodic_Filter_Take_Map_Subscribe(t *testing.T) {
	cnt := 0
	done := false
	stream.Pipe4(
		// just to demo composition
		stream.Pipe2(
			stream.Periodic(500*time.Millisecond),
			stream.Filter(func(i int) bool {
				return i%2 == 0
			}),
		),
		stream.Take[int](5),
		stream.Map(func(i int) int {
			return i * 2
		}),
		stream.Subscribe(stream.Observer[int]{
			Next: func(i int) bool {
				cnt++
				return true
			},
			Complete: func() {
				done = true
			},
		}),
	)

	time.Sleep(6 * time.Second)
	require.Equal(t, 5, cnt)
	require.True(t, done)
}

func TestOf_Periodic_Merge_Take_Map_ForEach(t *testing.T) {
	cnt := 0
	p := stream.Periodic(500 * time.Millisecond)
	f := stream.Of(10, 20)
	stream.Pipe4(
		stream.Merge(p, f),
		stream.Take[int](5),
		stream.Map(func(i int) int {
			return i * 2
		}),
		stream.ForEach(func(i int) {
			cnt++
			fmt.Println(i)
		}),
	)

	time.Sleep(3 * time.Second)
	require.Equal(t, 5, cnt)
}

func TestAccept_CollectSlice(t *testing.T) {
	cnt := 0
	done := false
	accept, s := stream.Accept[int]()
	cancel := stream.Pipe2(
		s,
		stream.Subscribe(stream.Observer[int]{
			Next: func(i int) bool {
				cnt++
				return true
			},
			Complete: func() {
				done = true
			},
		}),
	)

	go func() {
		for i := 0; i < 3; i++ {
			accept(i)
		}
	}()

	time.Sleep(1 * time.Second)
	require.Equal(t, 3, cnt)
	cancel()
	require.True(t, done)
}
