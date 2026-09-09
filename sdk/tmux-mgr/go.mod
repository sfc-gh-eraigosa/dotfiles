module github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr

go 1.26.1

require (
	github.com/sirupsen/logrus v1.10.1
	github.com/spf13/cobra v1.10.2
)

require (
	golang.org/x/sys v0.36.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/sfc-gh-eraigosa/dotfiles/sdk/libs v0.0.0
	github.com/spf13/pflag v1.0.9 // indirect
)

replace github.com/sfc-gh-eraigosa/dotfiles/sdk/libs => ../libs
