1. **Remove `boundedLogger` and implement Lumberjack rotation:**
   - In `cmd/svc/main.go`, replace `boundedLogger` references with `lumberjack.Logger`.
   - Update `initLogFiles()`: Resolve `os.Executable()`, create a `logs` directory alongside the binary (`0700` permission). Set `infoLogFilePath` and `errorLogFilePath` to `logs/access.log` and `logs/error.log`.
   - Configure `lumberjack.Logger` for both logs: MaxSize 10MB, MaxBackups 5, Compress true.
   - If log files already exist and have content, call `.Rotate()` to clear the current logs and save them as zip backups.

2. **Enhance Error/Warning Detection:**
   - In `singBoxLogWriter.Write()`, add `warning` to the list of keywords that trigger an `error` level log.

3. **Add System Status Recording:**
   - Add a `recordSystemStatus()` function that executes `netsh interface show interface` (on Windows) or `ip link show` (on Unix) and logs the output using `sysLogError()`.

4. **Fix Zombie Process Issue (Bug 2):**
   - In `monitorNetwork()`, call `recordSystemStatus()` when `failCount >= 3` before `p.cleanup()`.
   - In `run()`, when `sing-box` exits and `!p.stopping`, call `os.Exit(1)` to ensure the OS service manager restarts the `portal-svc` process automatically.

5. **Fix Misleading UAC Error (Bug 1):**
   - In `main()` CLI logic, update the error handling for `util.RunMeElevated()` so that it distinguishes between a true permission denied error and a non-zero exit code from the elevated process.
   - Update the `start` command to read the new `errorLogFilePath` when checking for post-start errors.

6. **Update Tests:**
   - In `cmd/svc/main_test.go`, remove tests related to `boundedLogger`.
   - Update `TestSingBoxLogWriter` to be compatible with `lumberjack.Logger` and the new path logic.

7. **Pre-commit Instructions:**
   - Run pre commit step to ensure proper testing, verification, review, and reflection are done.
