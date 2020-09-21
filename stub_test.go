package radix

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	. "testing"
	"time"

	"github.com/mediocregopher/radix/v3/resp"
	"github.com/mediocregopher/radix/v3/resp/resp2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Watching the watchmen

func testStub() Conn {
	m := map[string]string{}
	return Stub("tcp", "127.0.0.1:6379", func(args []string) interface{} {
		switch args[0] {
		case "GET":
			return m[args[1]]
		case "SET":
			m[args[1]] = args[2]
			return nil
		case "ECHO":
			return args[1]
		default:
			return fmt.Errorf("testStub doesn't support command %q", args[0])
		}
	})
}

func TestStub(t *T) {
	ctx := testCtx(t)
	stub := testStub()

	{ // Basic test
		var foo string
		require.Nil(t, stub.Do(ctx, Cmd(nil, "SET", "foo", "a")))
		require.Nil(t, stub.Do(ctx, Cmd(&foo, "GET", "foo")))
		assert.Equal(t, "a", foo)
	}

	{ // Basic test with an int, to ensure marshalling/unmarshalling all works
		var foo int
		require.Nil(t, stub.Do(ctx, FlatCmd(nil, "SET", "foo", 1)))
		require.Nil(t, stub.Do(ctx, Cmd(&foo, "GET", "foo")))
		assert.Equal(t, 1, foo)
	}
}

func TestStubPipeline(t *T) {
	ctx := testCtx(t)
	stub := testStub()

	var out string
	err := stub.Do(ctx, Pipeline(
		Cmd(nil, "SET", "foo", "bar"),
		Cmd(&out, "GET", "foo"),
	))

	require.Nil(t, err)
	assert.Equal(t, "bar", out)
}

func TestStubLockingTimeout(t *T) {
	ctx := testCtx(t)
	stub := testStub()
	wg := new(sync.WaitGroup)
	c := 1000

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < c; i++ {
			m := Cmd(nil, "ECHO", strconv.Itoa(i)).(resp.Marshaler)
			require.Nil(t, stub.EncodeDecode(ctx, m, nil))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < c; i++ {
			var j int
			require.Nil(t, stub.EncodeDecode(ctx, nil, resp2.Any{I: &j}))
			assert.Equal(t, i, j)
		}
	}()

	wg.Wait()

	// test out timeout. do a write-then-read to ensure nothing bad happens
	// when there's actually data to read
	now := time.Now()
	conn := stub.NetConn()
	conn.SetDeadline(now.Add(2 * time.Second))
	m := Cmd(nil, "ECHO", "1").(resp.Marshaler)
	require.Nil(t, stub.EncodeDecode(ctx, m, resp2.Any{}))

	// now there's no data to read, should return after 2-ish seconds with a
	// timeout error
	err := stub.EncodeDecode(ctx, nil, resp2.Any{})
	nerr, ok := err.(*net.OpError)
	assert.True(t, ok)
	assert.True(t, nerr.Timeout())
}

func ExampleStub() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := map[string]string{}
	stub := Stub("tcp", "127.0.0.1:6379", func(args []string) interface{} {
		switch args[0] {
		case "GET":
			return m[args[1]]
		case "SET":
			m[args[1]] = args[2]
			return nil
		default:
			return fmt.Errorf("this stub doesn't support command %q", args[0])
		}
	})

	stub.Do(ctx, Cmd(nil, "SET", "foo", "1"))

	var foo int
	stub.Do(ctx, Cmd(&foo, "GET", "foo"))
	fmt.Printf("foo: %d\n", foo)
}
