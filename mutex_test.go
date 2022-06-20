package confort

import (
	"context"
	"testing"
	"time"
)

func TestKeyedLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := newKeyedLock()

	timeout := func(t *testing.T, ctx context.Context, timeout time.Duration) context.Context {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		t.Cleanup(cancel)
		return ctx
	}

	t.Run("timeout on Lock during RLock", func(t *testing.T) {
		t.Parallel()

		key := t.Name()

		err := m.RLock(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		err = m.RLock(ctx, key)
		if err != nil {
			t.Fatal(err)
		}

		err = m.Lock(
			timeout(t, ctx, time.Millisecond*100),
			key,
		)
		if err == nil {
			t.Fatal("Lock succeeded unexpectedly")
		}

		m.RUnlock(key)

		err = m.Lock(
			timeout(t, ctx, time.Millisecond*100),
			key,
		)
		if err == nil {
			t.Fatal("Lock succeeded unexpectedly")
		}

		m.RUnlock(key)

		err = m.Lock(
			timeout(t, ctx, time.Millisecond*100),
			key,
		)
		if err != nil {
			t.Fatal(err)
		}
		m.Unlock(key)
	})

	t.Run("timeout on RLock during Lock", func(t *testing.T) {
		t.Parallel()

		key := t.Name()

		err := m.Lock(ctx, key)
		if err != nil {
			t.Fatal(err)
		}

		err = m.Lock(
			timeout(t, ctx, time.Millisecond*100),
			key,
		)
		if err == nil {
			t.Fatal("Lock succeeded unexpectedly")
		}

		err = m.RLock(
			timeout(t, ctx, time.Millisecond*100),
			key,
		)
		if err == nil {
			t.Fatal("RLock succeeded unexpectedly")
		}

		m.Unlock(key)

		err = m.RLock(
			timeout(t, ctx, time.Millisecond*100),
			key,
		)
		if err != nil {
			t.Fatal(err)
		}

		m.RUnlock(key)
	})

	t.Run("unlocked Unlock", func(t *testing.T) {
		t.Parallel()

		recovered := func() (r any) {
			defer func() {
				r = recover()
			}()
			m.Unlock(t.Name())
			return nil
		}()
		if recovered == nil {
			t.Fatal("unexpected success")
		}
	})

	t.Run("unlocked Downgrade", func(t *testing.T) {
		t.Parallel()

		recovered := func() (r any) {
			defer func() {
				r = recover()
			}()
			m.Downgrade(t.Name())
			return nil
		}()
		if recovered == nil {
			t.Fatal("unexpected success")
		}
	})

	t.Run("unlocked RUnlock", func(t *testing.T) {
		t.Parallel()

		recovered := func() (r any) {
			defer func() {
				r = recover()
			}()
			m.RUnlock(t.Name())
			return nil
		}()
		if recovered == nil {
			t.Fatal("unexpected success")
		}
	})
}

func BenchmarkKeyedLock(b *testing.B) {
	ctx := context.Background()
	m := newKeyedLock()

	key := b.Name()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%5 == 0 {
			err := m.Lock(ctx, key)
			if err != nil {
				b.Fatal(err)
			}
			time.Sleep(time.Millisecond * 50)
			m.Unlock(key)
		} else {
			err := m.RLock(ctx, key)
			if err != nil {
				b.Fatal(err)
			}
			go func() {
				time.Sleep(time.Millisecond * 75)
				m.RUnlock(key)
			}()
		}
	}
}
