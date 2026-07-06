package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestDispatchControl(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		r := dispatchControl(Request{Cmd: "status"}, nil)
		if !r.OK || r.PID == 0 {
			t.Errorf("status = %+v, want OK with a pid", r)
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		r := dispatchControl(Request{Cmd: "bogus"}, nil)
		if r.OK || !strings.Contains(r.Error, "unknown command") {
			t.Errorf("bogus = %+v, want OK=false with 'unknown command'", r)
		}
	})

	t.Run("reload not supported when reload is nil", func(t *testing.T) {
		r := dispatchControl(Request{Cmd: "reload"}, nil)
		if r.OK || !strings.Contains(r.Error, "not supported") {
			t.Errorf("reload(nil) = %+v, want OK=false 'reload not supported'", r)
		}
	})

	t.Run("check reload error", func(t *testing.T) {
		r := dispatchControl(Request{Cmd: "reload"}, func() error { return errors.New("boom") })
		if r.OK || r.Error != "boom" {
			t.Errorf("reload(err) = %+v, want OK=false error=boom", r)
		}
	})

	t.Run("reload ok", func(t *testing.T) {
		r := dispatchControl(Request{Cmd: "reload"}, func() error { return nil })
		if !r.OK || r.PID == 0 {
			t.Errorf("reload(ok) = %+v, want OK with a pid", r)
		}
	})
}
