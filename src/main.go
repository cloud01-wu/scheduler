package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/blockloop/scan"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/reugn/go-quartz/quartz"

	"github.com/cloud01-wu/cgsl/dbx"
	"github.com/cloud01-wu/cgsl/env"
	"github.com/cloud01-wu/cgsl/httpx/server"
	"github.com/cloud01-wu/cgsl/logger"
	v1 "github.com/cloud01-wu/scheduler/controllers/v1"
	"github.com/cloud01-wu/scheduler/global"
	"github.com/cloud01-wu/scheduler/helper"
	"github.com/cloud01-wu/scheduler/orm"
	"go.uber.org/zap"
)

var (
	// BuildTime when executing make command
	BuildTime string = "N/A"
	// Version of the built binary
	Version string = fmt.Sprintf("v1.0.0 %s", BuildTime)

	interrupt chan os.Signal
)

func initDatabase(
	endpoint string,
	username string,
	password string,
	dbName string,
	maxOpenConns int,
	maxIdleConns int,
	migrationFolder string,
	migrationVersion uint,
) error {
	var err error

	// initialize mysql connections
	err = dbx.Init(
		endpoint,
		username,
		password,
		dbName,
		maxOpenConns,
		maxIdleConns)

	if err != nil {
		return err
	}

	instance := dbx.New()
	driver, err := mysql.WithInstance(instance, &mysql.Config{})
	if err != nil {
		logger.New().Error(err.Error())
		return err
	}

	migrationFolder = strings.TrimSuffix(migrationFolder, string(filepath.Separator))
	logger.New().Debug("MIGRATION", zap.String("FOLDER", migrationFolder))
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationFolder),
		dbName,
		driver)

	if err != nil {
		return err
	} else {
		err := m.Migrate(migrationVersion)
		if err != nil {
			if err.Error() != "no change" {
				logger.New().Info("MIGRATION", zap.String("RESULT", err.Error()))
				return err
			}
		}
	}

	// test MySQL connection
	return dbx.New().Ping()
}

func restoreScheduleJobs(scheduler quartz.Scheduler) error {
	stmt1, err := dbx.New().Prepare(
		`
		SELECT * 
		FROM schedule_jobs 
		;
	`)
	if err != nil {
		return err
	}
	defer stmt1.Close()

	rows1, err := stmt1.Query()
	if err != nil {
		return err
	}
	defer rows1.Close()

	scheduleJobs := []orm.ScheduleJob{}
	err = scan.Rows(&scheduleJobs, rows1)
	if err != nil {
		return err
	}

	for _, scheduleJob := range scheduleJobs {
		if scheduleJob.Status != orm.JobStatusEnable {
			continue
		}

		job, err := helper.NewJob(
			scheduler,
			scheduleJob.JobID,
			scheduleJob.Name,
			scheduleJob.TriggerType,
			scheduleJob.Expression,
			scheduleJob.HttpMethod,
			scheduleJob.HttpTargetUrl,
			scheduleJob.HttpRequestBody,
			scheduleJob.JsonWebToken,
		)
		if err != nil {
			break
		}

		logger.New().Debug("SCHEDULE JOB WAS RESTORED", zap.String("JobID", scheduleJob.JobID), zap.Int("JobKey", job.Key()), zap.String("Name", scheduleJob.Name))

		stmt2, err := dbx.New().Prepare(
			`
			UPDATE schedule_jobs SET JobKey=? WHERE JobID=?
			;
		`)
		if err != nil {
			return err
		}
		defer stmt2.Close()

		_, err = stmt2.Exec(job.Key(), scheduleJob.JobID)
		if err != nil {
			return err
		}
	}

	return err
}

func main() {
	var (
		showHelp    bool
		showVersion bool
		err         error
	)

	flag.BoolVar(&showHelp, "h", false, "help (optional)")
	flag.BoolVar(&showVersion, "v", false, "version (optional)")
	flag.Parse()

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	// find current working directory
	executable, err := os.Executable()
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	directory := filepath.Dir(executable)

	// configure log
	logger.SetEnvParam(
		directory+string(os.PathSeparator)+"logs",
		10,
		"debug")

	// fetch log configuration from environment variable
	httpBindAddr := env.GetString("HTTP_BIND_ADDR", "0.0.0.0")
	httpPort := env.GetInt("HTTP_PORT", 80)
	dbEndpoint := env.GetString("DB_ENDPOINT", "")
	dbName := env.GetString("DB_NAME", "SCHEDULER")
	dbUsername := env.GetString("DB_USERNAME", "")
	dbPassword := env.GetString("DB_PASSWORD", "")
	dbMaxOpenConns := env.GetInt("DB_MAX_OPEN_CONNS", 25)
	dbMaxIdleConns := env.GetInt("DB_MAX_IDLE_CONNS", 5)
	dbMigrationsFolder := env.GetString("DB_MIGRATIONS_FOLDER", "/opt/db/migrations")
	dbMigrationsVersion := env.GetUint("DB_MIGRATIONS_VERSION", 1)

	// initialize database
	err = initDatabase(
		dbEndpoint,
		dbUsername,
		dbPassword,
		dbName,
		dbMaxOpenConns,
		dbMaxIdleConns,
		dbMigrationsFolder,
		dbMigrationsVersion,
	)

	if err != nil {
		logger.New().Error("FAILED TO INITIALIZE DATABASE", zap.Error(err))
		os.Exit(1)
	}

	// initialize go-quartz
	global.Scheduler = quartz.NewStdScheduler()
	global.Scheduler.Start(context.Background())

	// restore schedule jobs with SQLite3
	err = restoreScheduleJobs(global.Scheduler)
	if err != nil {
		logger.New().Error("FAILED TO RESTORE SCHEDULE JOB(S)", zap.Error(err))
		os.Exit(1)
	}

	// register os signal
	interrupt = make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	httpServer := server.New(httpBindAddr, httpPort)
	global.HttpServer = httpServer

	httpServer.RegisterAPI("scheduler.v1.post.job", "POST", "/api/v1/jobs", v1.PostJob)
	httpServer.RegisterAPI("scheduler.v1.get.jobs", "GET", "/api/v1/jobs", v1.GetJobs)
	httpServer.RegisterAPI("scheduler.v1.get.job", "GET", "/api/v1/jobs/{jobID}", v1.GetJob)
	httpServer.RegisterAPI("scheduler.v1.put.job", "PUT", "/api/v1/jobs/{jobID}", v1.PutJob)
	httpServer.RegisterAPI("scheduler.v1.delete.job", "DELETE", "/api/v1/jobs/{jobID}", v1.DeleteJob)
	httpServer.RegisterAPI("scheduler.v1.delete.jobs", "DELETE", "/api/v1/jobs", v1.DeleteJobs)

	// start HTTP server
	httpServer.Start()
	logger.New().Warn("HTTP SERVER STARTED", zap.String("host", httpBindAddr), zap.Int("port", httpPort))

	// enter service loop
	sig := <-interrupt
	logger.New().Warn("TERMINATING...", zap.String("signal", sig.String()))

	// shutdown HTTP server
	httpServer.Stop()
	logger.New().Info("HTTP SERVER EXITED")

	global.Scheduler.Stop()
	global.Scheduler.Wait(context.Background())
	logger.New().Info("SCHEDULER EXITED")
}
