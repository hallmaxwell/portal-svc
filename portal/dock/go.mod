module hawego/portal/dock

go 1.26.1

require (
	github.com/kardianos/service v1.2.4
	github.com/nxadm/tail v1.4.11
)

require (
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	hawego/portal/internal/template v0.0.0-00010101000000-000000000000 // indirect
)

replace hawego/portal/internal/template => ../internal/template
