Output demo.gif

Set Theme "Catppuccin Mocha"
Set Width 1200
Set Height 800
Set Padding 20
Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Margin 20
Set MarginFill "#1e1e2e"
Set BorderRadius 10
Set WindowBar Rings

# Create a fake env so setup doesn't trigger automatically if we don't want to
Hide
Type 'touch .env'
Enter
Type 'echo "DO_IP=1.1.1.1" > .env'
Enter
Type 'echo "UUID=123" >> .env'
Enter
Show

Type "./portal-svc"
Enter

Sleep 2s

Type "setup"
Sleep 1s
Enter

Sleep 2s
Type "192.168.1.100"
Sleep 1s
Enter
Sleep 1s

Type "test-uuid"
Sleep 1s
Enter
Sleep 1s

Type "test-pubkey"
Sleep 1s
Enter
Sleep 1s

Type "test-shortid"
Sleep 1s
Enter
Sleep 1s

Type '["test.com"]'
Sleep 1s
Enter
Sleep 1s

Type "y"
Sleep 1s
Enter
Sleep 2s

Type "exit"
Sleep 1s
Enter
Sleep 1s
