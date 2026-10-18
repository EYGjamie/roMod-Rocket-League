//go:build windows

package desktop

import (
	"errors"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	// Task Manager and Settings switch entries off here without removing them.
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	runValue    = "auto-replay-upload"
	// remembers whether the user turned autostart off in the menu
	settingsKey   = `Software\auto-replay-upload`
	settingsValue = "Autostart"
)

// HasAutostart reports that starting at sign-in can be switched.
const HasAutostart = true

// Autostart reports whether Windows starts the tool at sign-in.
func Autostart() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(runValue); err != nil {
		return false
	}
	return !switchedOff()
}

func switchedOff() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(runValue)
	return err == nil && len(b) > 0 && b[0]&1 != 0 // 2 = on, 3 = off
}

// SyncAutostart applies the remembered choice, which is on until the user
// turns it off, and keeps the path in args current in case the folder moved.
func SyncAutostart(args []string) error {
	on := true
	if k, err := registry.OpenKey(registry.CURRENT_USER, settingsKey, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetIntegerValue(settingsValue); err == nil {
			on = v != 0
		}
		k.Close()
	}
	return applyAutostart(on, args)
}

// SetAutostart remembers the choice and makes Windows run args at sign-in,
// or stops that. Turning it on also undoes switching it off in the Task
// Manager.
func SetAutostart(on bool, args []string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, settingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	v := uint32(0)
	if on {
		v = 1
	}
	err = k.SetDWordValue(settingsValue, v)
	k.Close()
	if err != nil {
		return err
	}
	if on {
		if a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE); err == nil {
			a.DeleteValue(runValue)
			a.Close()
		}
	}
	return applyAutostart(on, args)
}

func applyAutostart(on bool, args []string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if on {
		return k.SetStringValue(runValue, windows.ComposeCommandLine(args))
	}
	if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
