module github.com/mediocregopher/radix/bench

go 1.14

require (
	github.com/garyburd/redigo v1.6.0 // indirect
	github.com/go-redis/redis v6.15.2+incompatible
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/joomcode/errorx v0.8.0 // indirect
	github.com/joomcode/redispipe v0.9.0
	github.com/mediocregopher/radix.v2 v0.0.0-20181115013041-b67df6e626f9 // indirect
	github.com/mediocregopher/radix/v3 v3.5.2
	github.com/mediocregopher/radix/v4 v4.0.0
	github.com/onsi/ginkgo v1.14.2 // indirect
	github.com/onsi/gomega v1.10.4 // indirect
)

replace github.com/mediocregopher/radix/v4 => ../.
