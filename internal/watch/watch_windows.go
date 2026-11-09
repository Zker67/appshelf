//go:build windows

package watch

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Watch calls changed, from a goroutine of its own, each time one of the
// keys or directories changes, until ctx ends. Keys and directories that
// cannot be opened, as those that do not exist, are left out; it returns how
// many it watches, and an error when it watches none.
func Watch(ctx context.Context, keys []Key, dirs []string, changed func()) (int, error) {
	ready := make(chan int, 1)
	go run(ctx, keys, dirs, changed, ready)
	n := <-ready
	if n == 0 {
		return 0, errors.New("没有可以监听的注册表键或目录")
	}
	return n, nil
}

type watched struct {
	event windows.Handle
	key   registry.Key // zero for a directory
	k     Key
}

func run(ctx context.Context, keys []Key, dirs []string, changed func(), ready chan<- int) {
	// The registrations live on this thread; with Windows 8 and later they
	// would not need it, but the thread costs nothing while it waits.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var all []watched
	defer func() {
		for _, w := range all {
			if w.key != 0 {
				w.key.Close()
				windows.CloseHandle(w.event)
			} else {
				windows.FindCloseChangeNotification(w.event)
			}
		}
	}()
	for _, k := range keys {
		w, err := openKey(k)
		if err != nil {
			continue
		}
		all = append(all, w)
	}
	for _, d := range dirs {
		h, err := windows.FindFirstChangeNotification(d, false, windows.FILE_NOTIFY_CHANGE_DIR_NAME)
		if err != nil {
			continue
		}
		all = append(all, watched{event: h})
	}
	ready <- len(all)
	if len(all) == 0 {
		return
	}

	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return
	}
	defer windows.CloseHandle(stop)
	go func() {
		<-ctx.Done()
		windows.SetEvent(stop)
	}()
	handles := make([]windows.Handle, 0, len(all)+1)
	handles = append(handles, stop)
	for _, w := range all {
		handles = append(handles, w.event)
	}
	for {
		i, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			return
		}
		i -= windows.WAIT_OBJECT_0
		if i == 0 || int(i) >= len(handles) {
			return
		}
		w := all[i-1]
		if w.key != 0 {
			if notify(w) != nil {
				return
			}
		} else if windows.FindNextChangeNotification(w.event) != nil {
			return
		}
		changed()
	}
}

func openKey(k Key) (watched, error) {
	root, access := registry.CURRENT_USER, uint32(registry.NOTIFY)
	if k.Machine {
		root = registry.LOCAL_MACHINE
		if k.X86 {
			access |= registry.WOW64_32KEY
		} else {
			access |= registry.WOW64_64KEY
		}
	}
	key, err := registry.OpenKey(root, k.Path, access)
	if err != nil {
		return watched{}, err
	}
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		key.Close()
		return watched{}, err
	}
	w := watched{event: ev, key: key, k: k}
	if err := notify(w); err != nil {
		key.Close()
		windows.CloseHandle(ev)
		return watched{}, err
	}
	return w, nil
}

// notify asks for the next change of a key: Windows signals a key's event
// once per request.
func notify(w watched) error {
	filter := uint32(windows.REG_NOTIFY_CHANGE_NAME | windows.REG_NOTIFY_THREAD_AGNOSTIC)
	if w.k.Values {
		filter |= windows.REG_NOTIFY_CHANGE_LAST_SET
	}
	if err := windows.RegNotifyChangeKeyValue(windows.Handle(w.key), w.k.Subtree, filter, w.event, true); err != nil {
		return fmt.Errorf("无法监听注册表：%w", err)
	}
	return nil
}
