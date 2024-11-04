package helper

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloud01-wu/cgsl/dbx"
	"github.com/cloud01-wu/cgsl/httpx/client"
	"github.com/cloud01-wu/cgsl/logger"
	"github.com/cloud01-wu/scheduler/global"
	"github.com/cloud01-wu/scheduler/orm"
	"github.com/reugn/go-quartz/quartz"
	"go.uber.org/zap"
)

func NewJob(
	scheduler quartz.Scheduler,
	jobID string,
	name string,
	triggerType string,
	expression string,
	httpMethod string,
	httpTargetUrl string,
	httpRequestPayload string,
	jsonWebToken string) (quartz.Job, error) {
	var (
		err     error
		trigger quartz.Trigger
	)

	switch triggerType {
	case "cron":
		trigger, err = quartz.NewCronTrigger(expression)
		if err != nil {
			return nil, err
		}
	case "interval":
		seconds, err := strconv.ParseInt(expression, 10, 64)
		if err != nil {
			return nil, err
		}

		trigger = quartz.NewSimpleTrigger(time.Second * time.Duration(seconds))
	case "once":
		seconds, err := strconv.ParseInt(expression, 10, 64)
		if err != nil {
			return nil, err
		}

		trigger = quartz.NewRunOnceTrigger(time.Second * time.Duration(seconds))
	default:
		return nil, err
	}

	headers := map[string]string{}
	if jsonWebToken != "" {
		headers["Authorization"] = "Bearer " + jsonWebToken
	}

	job := quartz.NewFunctionJob(func(_ context.Context) (int, error) {
		// update job status
		if triggerType == "once" {
			stmt, err := dbx.New().Prepare(`
				UPDATE schedule_jobs SET Status=? WHERE JobID=?;
			`)
			if err != nil {
				logger.New().Error("FAILED TO UPDATE JOB STATUS", zap.String("JobID", jobID), zap.String("Name", name), zap.Error(err))
				return -1, err
			}
			defer stmt.Close()

			_, err = stmt.Exec(orm.JobStatusDone, jobID)
			if err != nil {
				logger.New().Error("FAILED TO UPDATE JOB STATUS", zap.String("JobID", jobID), zap.String("Name", name), zap.Error(err))
				return -1, err
			}
		}

		// exec job
		urlObject, err := url.Parse(httpTargetUrl)
		if err != nil {
			logger.New().Error("FAILED TO PARSE TARGET URL", zap.String("JobID", jobID), zap.String("Name", name), zap.Error(err))
			return -1, err
		}

		// enable secure while HTTP scheme is "https"
		secureEnable := false
		if strings.EqualFold(urlObject.Scheme, "https") {
			secureEnable = true
		}

		httpClient, err := client.New(urlObject.Host, &client.Options{
			Secure: secureEnable,
		})
		if err != nil {
			logger.New().Error("FAILED TO INIT HTTP CLIENT", zap.String("JobID", jobID), zap.String("Name", name), zap.Error(err))
			return -1, err
		}

		payloadData := []byte(httpRequestPayload)
		res, err := httpClient.ExecuteMethod(
			context.Background(),
			httpMethod,
			strings.TrimPrefix(urlObject.Path, "/"), // trim leading slash
			client.RequestMetadata{
				QueryValues:   urlObject.Query(),
				Headers:       headers,
				ContentType:   "application/octet-stream",
				ContentLength: len(payloadData),
				ContentBody:   bytes.NewReader(payloadData),
			},
		)
		if err != nil {
			logger.New().Error("FAILED EXECUTE METHOD", zap.String("JobID", jobID), zap.String("Name", name), zap.Error(err))
			return -1, err
		}

		if res.StatusCode >= 400 {
			builder := new(strings.Builder)
			io.Copy(builder, res.Body)
			logger.New().Info("FETCH FAILURE HTTP STATUS CODE", zap.String("JobID", jobID), zap.String("Name", name), zap.String("Response", builder.String()))
		} else {
			logger.New().Info("JOB ACCOMPLISHED", zap.String("JobID", jobID), zap.String("Name", name))
		}

		return res.StatusCode, nil
	})

	if err != nil {
		return nil, err
	}

	err = global.Scheduler.ScheduleJob(context.Background(), job, trigger)
	if err != nil {
		return nil, err
	}

	return job, nil
}
