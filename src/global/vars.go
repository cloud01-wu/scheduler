package global

import (
	"github.com/cloud01-wu/cgsl/httpx/server"
	"github.com/reugn/go-quartz/quartz"
)

var (
	HttpServer *server.Server
	Scheduler  quartz.Scheduler
)
