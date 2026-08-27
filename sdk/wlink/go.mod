module github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink

go 1.26.3

require (
	github.com/sfc-gh-eraigosa/dotfiles/sdk/libs v0.0.0-00010101000000-000000000000
	github.com/sirupsen/logrus v1.10.1
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.13.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

replace github.com/sfc-gh-eraigosa/dotfiles/sdk/libs => ../libs
