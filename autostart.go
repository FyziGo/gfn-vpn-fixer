//go:build windows

package main

import (
	"fmt"
	"os"
)

// isAutostartEnabled returns true if the GFNNetWrapper scheduled task exists.
func isAutostartEnabled() bool {
	_, err := runPS("Get-ScheduledTask -TaskName 'GFNNetWrapper' -ErrorAction Stop")
	return err == nil
}

// setAutostart creates or removes a Task Scheduler entry that launches the
// executable at user logon with highest privileges — no UAC prompt is needed.
func setAutostart(enable bool) error {
	// Remove legacy HKCU\...\Run entry if it was created by an older version.
	_, _ = runPS("Remove-ItemProperty -Path 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run' " +
		"-Name 'GFNNetWrapper' -ErrorAction SilentlyContinue")

	if enable {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("не удалось получить путь к exe: %w", err)
		}

		psCmd := fmt.Sprintf(
			"$a = New-ScheduledTaskAction -Execute '%s'; "+
				"$t = New-ScheduledTaskTrigger -AtLogOn -User ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name); "+
				"$p = New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) "+
				"-RunLevel Highest -LogonType Interactive; "+
				"$s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries "+
				"-ExecutionTimeLimit (New-TimeSpan); "+
				"Register-ScheduledTask -TaskName 'GFNNetWrapper' -Action $a -Trigger $t "+
				"-Principal $p -Settings $s -Force",
			psEscape(exe),
		)

		if _, err := runPS(psCmd); err != nil {
			return fmt.Errorf("не удалось создать задачу планировщика: %w", err)
		}
		return nil
	}

	// Disable: remove the scheduled task (ignore "not found").
	_, _ = runPS("Unregister-ScheduledTask -TaskName 'GFNNetWrapper' -Confirm:$false -ErrorAction SilentlyContinue")
	return nil
}
