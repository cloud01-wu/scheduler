package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/blockloop/scan"
	"github.com/cloud01-wu/cgsl/datetime"
	"github.com/cloud01-wu/cgsl/dbx"
	"github.com/cloud01-wu/cgsl/httpx/model"
	"github.com/cloud01-wu/cgsl/logger"
	"github.com/cloud01-wu/cgsl/utils"
	"github.com/cloud01-wu/scheduler/global"
	"github.com/cloud01-wu/scheduler/helper"
	"github.com/cloud01-wu/scheduler/orm"
	"github.com/gorilla/mux"
	"github.com/reugn/go-quartz/quartz"
	"go.uber.org/zap"
)

type PostJobRequest struct {
	Name            string `json:"name" valid:"stringlength(1|32)"`
	TriggerType     string `json:"triggerType" valid:"in(cron|interval|once)"`
	Expression      string `json:"expression" valid:"expression~expression does not validate as specific cron expression. See https://github.com/reugn/go-quartz"`
	HttpMethod      string `json:"httpMethod" valid:"in(POST|GET|PUT|DELETE)"`
	HttpTargetUrl   string `json:"httpTargetUrl" valid:"requrl~httpTargetUrl does not validate as valid HTTP request URL"`
	HttpRequestBody string `json:"httpRequestBody" valid:"-"`
	JsonWebToken    string `json:"jsonWebToken" valid:"-"`
}

type PutJobRequest struct {
	Status          int    `json:"status" valid:"range(1|2)~status must be 1 (enable) or 2 (disable)"`
	Name            string `json:"name" valid:"stringlength(1|32)"`
	TriggerType     string `json:"triggerType" valid:"in(cron|interval|once)"`
	Expression      string `json:"expression" valid:"expression~expression does not validate as specific cron/interval/once expression. See https://github.com/reugn/go-quartz"`
	HttpMethod      string `json:"httpMethod" valid:"in(POST|GET|PUT|DELETE)"`
	HttpTargetUrl   string `json:"httpTargetUrl" valid:"requrl~httpTargetUrl does not validate as valid HTTP request URL"`
	HttpRequestBody string `json:"httpRequestBody" valid:"-"`
	JsonWebToken    string `json:"jsonWebToken" valid:"-"`
}

type GetJobResult struct {
	JobID           string `json:"jobId"`
	JobKey          int    `json:"jobKey"`
	Status          int    `json:"status"`
	Name            string `json:"name"`
	TriggerType     string `json:"triggerType"`
	Expression      string `json:"expression"`
	HttpMethod      string `json:"httpMethod"`
	HttpTargetUrl   string `json:"httpTargetUrl"`
	HttpRequestBody string `json:"httpRequestBody"`
	JsonWebToken    string `json:"jsonWebToken"`
	CreationTime    string `json:"creationTime"`
	UpdateTime      string `json:"updateTime"`
}

func init() {
	govalidator.SetFieldsRequiredByDefault(true)
	govalidator.TagMap["expression"] = govalidator.Validator(func(expression string) bool {
		_, err := quartz.NewCronTrigger(expression)
		if err != nil {
			_, err = strconv.ParseInt(expression, 10, 64)
		}
		return err == nil
	})
}

func PostJob(w http.ResponseWriter, r *http.Request) {
	var (
		params       = map[string]interface{}{}
		resultObject = model.Response{}
	)

	// receive post data
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, err),
		))
		return
	}

	// deserialize data
	requestObject := model.Request{
		Desire: nil,
		Data:   &PostJobRequest{},
	}

	err = json.Unmarshal(body, &requestObject)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, err),
		))
		return
	}

	requestData, ok := requestObject.Data.(*PostJobRequest)
	if !ok {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, errors.New("unexpected request data")),
		))
		return
	}

	_, err = govalidator.ValidateStruct(requestData)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, err),
		))
		return
	}

	params["HttpBody"] = requestData

	now := datetime.Now()
	jobID := utils.RandomUUIDString()
	scheduleJob := orm.ScheduleJob{
		JobID:           jobID,
		Status:          1,
		Name:            requestData.Name,
		TriggerType:     requestData.TriggerType,
		Expression:      requestData.Expression,
		HttpMethod:      requestData.HttpMethod,
		HttpTargetUrl:   requestData.HttpTargetUrl,
		HttpRequestBody: requestData.HttpRequestBody,
		JsonWebToken:    requestData.JsonWebToken,
		CreationTime:    now.EpochInSecond(),
		UpdateTime:      now.EpochInSecond(),
	}

	job, err := helper.NewJob(
		global.Scheduler,
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
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, errors.New("invalid argument(s): "+err.Error())),
		))
		return
	}
	scheduleJob.JobKey = job.Key()

	// insert job into database
	stmt1, err := dbx.New().Prepare(`
		INSERT INTO schedule_jobs (JobID,JobKey,Status,Name,TriggerType,Expression,HttpMethod,HttpTargetUrl,HttpRequestBody,JsonWebToken,CreationTime,UpdateTime) 
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		;
	`)

	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt1.Close()

	_, err = stmt1.Exec(
		scheduleJob.JobID,
		scheduleJob.JobKey,
		scheduleJob.Status,
		scheduleJob.Name,
		scheduleJob.TriggerType,
		scheduleJob.Expression,
		scheduleJob.HttpMethod,
		scheduleJob.HttpTargetUrl,
		scheduleJob.HttpRequestBody,
		scheduleJob.JsonWebToken,
		scheduleJob.CreationTime,
		scheduleJob.UpdateTime,
	)

	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	resultObject.Data = &GetJobResult{
		JobID:           scheduleJob.JobID,
		JobKey:          scheduleJob.JobKey,
		Status:          scheduleJob.Status,
		Name:            scheduleJob.Name,
		TriggerType:     scheduleJob.TriggerType,
		Expression:      scheduleJob.Expression,
		HttpMethod:      scheduleJob.HttpMethod,
		HttpTargetUrl:   scheduleJob.HttpTargetUrl,
		HttpRequestBody: scheduleJob.HttpRequestBody,
		JsonWebToken:    scheduleJob.JsonWebToken,
		CreationTime:    datetime.FromUnixTime(scheduleJob.CreationTime).String(),
		UpdateTime:      datetime.FromUnixTime(scheduleJob.UpdateTime).String(),
	}

	logger.New().Info(utils.CurrentFunctionName(), zap.Any("Params", params))
	result, err := json.Marshal(resultObject)
	if nil != err {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}
}

func GetJobs(w http.ResponseWriter, r *http.Request) {
	var (
		params       = map[string]interface{}{}
		resultObject = model.Response{}
	)

	query := r.URL.Query()
	from := GetIntFromQuery(query, "from", 0)
	size := GetIntFromQuery(query, "size", 0)
	params["From"] = from
	params["Size"] = size

	withLimit := false
	if size != 0 {
		withLimit = true
	}

	stmt1, err := dbx.New().Prepare(`
		SELECT COUNT(*) AS TotalCount 
		FROM schedule_jobs
		;
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt1.Close()

	rows1, err := stmt1.Query()
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer rows1.Close()

	totalCount := 0
	err = scan.Row(&totalCount, rows1)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	arguments2 := []interface{}{}
	stmt2String := `
		SELECT * 
		FROM schedule_jobs 
	`
	if withLimit {
		stmt2String += `LIMIT ?,?`
		arguments2 = append(arguments2, from, size)
	}

	stmt2, err := dbx.New().Prepare(stmt2String)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt2.Close()

	rows2, err := stmt2.Query(arguments2...)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer rows2.Close()

	scheduleJobs := []orm.ScheduleJob{}
	err = scan.Rows(&scheduleJobs, rows2)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	entities := []*GetJobResult{}
	for _, scheduleJob := range scheduleJobs {
		entities = append(entities, &GetJobResult{
			JobID:           scheduleJob.JobID,
			JobKey:          scheduleJob.JobKey,
			Status:          scheduleJob.Status,
			Name:            scheduleJob.Name,
			TriggerType:     scheduleJob.TriggerType,
			Expression:      scheduleJob.Expression,
			HttpMethod:      scheduleJob.HttpMethod,
			HttpTargetUrl:   scheduleJob.HttpTargetUrl,
			HttpRequestBody: scheduleJob.HttpRequestBody,
			JsonWebToken:    scheduleJob.JsonWebToken,
			CreationTime:    datetime.FromUnixTime(scheduleJob.CreationTime).String(),
			UpdateTime:      datetime.FromUnixTime(scheduleJob.UpdateTime).String(),
		})
	}

	resultObject.Meta = &model.Meta{
		From:  from,
		Size:  len(entities),
		Total: totalCount,
	}
	resultObject.Data = entities

	logger.New().Info(utils.CurrentFunctionName(), zap.Any("Params", params))
	result, err := json.Marshal(resultObject)
	if nil != err {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}
}

func GetJob(w http.ResponseWriter, r *http.Request) {
	var (
		params       = map[string]interface{}{}
		resultObject = model.Response{}
	)

	vars := mux.Vars(r)
	jobID := vars["jobID"]

	params["JobID"] = jobID

	if !govalidator.IsUUIDv4(jobID) {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, errors.New("invalid job UUID")),
		))
		return
	}

	stmt1, err := dbx.New().Prepare(`
		SELECT * 
		FROM schedule_jobs 
		WHERE JobID=?
		;
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt1.Close()

	rows1, err := stmt1.Query(jobID)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer rows1.Close()

	scheduleJob := orm.ScheduleJob{}
	err = scan.Row(&scheduleJob, rows1)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	resultObject.Data = &GetJobResult{
		JobID:           scheduleJob.JobID,
		JobKey:          scheduleJob.JobKey,
		Status:          scheduleJob.Status,
		Name:            scheduleJob.Name,
		TriggerType:     scheduleJob.TriggerType,
		Expression:      scheduleJob.Expression,
		HttpMethod:      scheduleJob.HttpMethod,
		HttpTargetUrl:   scheduleJob.HttpTargetUrl,
		HttpRequestBody: scheduleJob.HttpRequestBody,
		JsonWebToken:    scheduleJob.JsonWebToken,
		CreationTime:    datetime.FromUnixTime(scheduleJob.CreationTime).String(),
		UpdateTime:      datetime.FromUnixTime(scheduleJob.UpdateTime).String(),
	}

	logger.New().Info(utils.CurrentFunctionName(), zap.Any("Params", params))
	result, err := json.Marshal(resultObject)
	if nil != err {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}
}

func PutJob(w http.ResponseWriter, r *http.Request) {
	var (
		params       = map[string]interface{}{}
		resultObject = model.Response{}
	)

	vars := mux.Vars(r)
	jobID := vars["jobID"]

	// receive post data
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, err),
		))
		return
	}

	// deserialize data
	requestObject := model.Request{
		Desire: &PutJobRequest{},
		Data:   nil,
	}

	err = json.Unmarshal(body, &requestObject)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, err),
		))
		return
	}

	requestData, ok := requestObject.Desire.(*PutJobRequest)
	if !ok {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, errors.New("unexpected request data")),
		))
		return
	}

	params["JobID"] = jobID
	params["HttpBody"] = requestData

	if !govalidator.IsUUIDv4(jobID) {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, errors.New("invalid job UUID")),
		))
		return
	}

	_, err = govalidator.ValidateStruct(requestData)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusBadRequest, err),
		))
		return
	}

	// query schedule job row
	stmt1, err := dbx.New().Prepare(`
		SELECT * 
		FROM schedule_jobs 
		WHERE JobID=?
		;
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt1.Close()

	rows1, err := stmt1.Query(jobID)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer rows1.Close()

	scheduleJob := orm.ScheduleJob{}
	err = scan.Row(&scheduleJob, rows1)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	// try to destory existing job
	_, err = global.Scheduler.GetScheduledJob(scheduleJob.JobKey)
	if err == nil {
		err = global.Scheduler.DeleteJob(scheduleJob.JobKey)
		if err != nil {
			logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
				responseError(w, &resultObject, http.StatusInternalServerError, err),
			))
			return
		}
	}
	scheduleJob.JobKey = -1

	if requestData.Status == 1 {
		// restore the job
		job, err := helper.NewJob(
			global.Scheduler,
			scheduleJob.JobID,
			requestData.Name,
			requestData.TriggerType,
			requestData.Expression,
			requestData.HttpMethod,
			requestData.HttpTargetUrl,
			requestData.HttpRequestBody,
			requestData.JsonWebToken,
		)
		if err != nil {
			logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
				responseError(w, &resultObject, http.StatusInternalServerError, err),
			))
			return
		}

		scheduleJob.JobKey = job.Key()
	}

	scheduleJob.Status = requestData.Status
	scheduleJob.Name = requestData.Name
	scheduleJob.TriggerType = requestData.TriggerType
	scheduleJob.Expression = requestData.Expression
	scheduleJob.HttpMethod = requestData.HttpMethod
	scheduleJob.HttpTargetUrl = requestData.HttpTargetUrl
	scheduleJob.HttpRequestBody = requestData.HttpRequestBody
	scheduleJob.JsonWebToken = requestData.JsonWebToken
	scheduleJob.UpdateTime = datetime.Now().EpochInSecond()

	stmt2, err := dbx.New().Prepare(`
		UPDATE schedule_jobs SET 
		JobKey=?,
		Status=?,
		Name=?,
		TriggerType=?,
		Expression=?,
		HttpMethod=?,
		HttpTargetUrl=?,
		HttpRequestBody=?,
		JsonWebToken=?,
		UpdateTime=? 
		WHERE JobID=?
		;
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt2.Close()

	_, err = stmt2.Exec(
		scheduleJob.JobKey,
		scheduleJob.Status,
		scheduleJob.Name,
		scheduleJob.TriggerType,
		scheduleJob.Expression,
		scheduleJob.HttpMethod,
		scheduleJob.HttpTargetUrl,
		scheduleJob.HttpRequestBody,
		scheduleJob.JsonWebToken,
		scheduleJob.UpdateTime,
		scheduleJob.JobID,
	)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	resultObject.Data = &GetJobResult{
		JobID:           scheduleJob.JobID,
		JobKey:          scheduleJob.JobKey,
		Status:          scheduleJob.Status,
		Name:            scheduleJob.Name,
		TriggerType:     scheduleJob.TriggerType,
		Expression:      scheduleJob.Expression,
		HttpMethod:      scheduleJob.HttpMethod,
		HttpTargetUrl:   scheduleJob.HttpTargetUrl,
		HttpRequestBody: scheduleJob.HttpRequestBody,
		JsonWebToken:    scheduleJob.JsonWebToken,
		CreationTime:    datetime.FromUnixTime(scheduleJob.CreationTime).String(),
		UpdateTime:      datetime.FromUnixTime(scheduleJob.UpdateTime).String(),
	}

	logger.New().Info(utils.CurrentFunctionName(), zap.Any("Params", params))
	result, err := json.Marshal(resultObject)
	if nil != err {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}
}

func DeleteJobs(w http.ResponseWriter, r *http.Request) {
	var (
		params       = map[string]interface{}{}
		resultObject = model.Response{}
	)

	// delete row
	stmt1, err := dbx.New().Prepare(`
		TRUNCATE TABLE schedule_jobs
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt1.Close()

	_, err = stmt1.Exec()
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	// destory existing jobs
	global.Scheduler.Clear()

	logger.New().Info(utils.CurrentFunctionName(), zap.Any("Params", params))
	result, err := json.Marshal(resultObject)
	if nil != err {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}
}

func DeleteJob(w http.ResponseWriter, r *http.Request) {
	var (
		params       = map[string]interface{}{}
		resultObject = model.Response{}
	)

	vars := mux.Vars(r)
	jobID := vars["jobID"]

	params["JobID"] = jobID

	if !govalidator.IsUUIDv4(jobID) {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, errors.New("invalid job UUID")),
		))
		return
	}

	// query schedule job row
	stmt1, err := dbx.New().Prepare(`
		SELECT * 
		FROM schedule_jobs 
		WHERE JobID=?
		;
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt1.Close()

	rows1, err := stmt1.Query(jobID)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer rows1.Close()

	scheduleJob := orm.ScheduleJob{}
	err = scan.Row(&scheduleJob, rows1)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	// try to destory existing job
	_, err = global.Scheduler.GetScheduledJob(scheduleJob.JobKey)
	if err == nil {
		err = global.Scheduler.DeleteJob(scheduleJob.JobKey)
		if err != nil {
			logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
				responseError(w, &resultObject, http.StatusInternalServerError, err),
			))
			return
		}
	}

	// delete row
	stmt2, err := dbx.New().Prepare(`
		DELETE  
		FROM schedule_jobs 
		WHERE JobID=?
	`)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}
	defer stmt2.Close()

	_, err = stmt2.Exec(scheduleJob.JobID)
	if err != nil {
		logger.New().Error(utils.CurrentFunctionName(), zap.Any("Params", params), zap.Error(
			responseError(w, &resultObject, http.StatusInternalServerError, err),
		))
		return
	}

	logger.New().Info(utils.CurrentFunctionName(), zap.Any("Params", params))
	result, err := json.Marshal(resultObject)
	if nil != err {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}
}
