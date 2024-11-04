package orm

type ScheduleJob struct {
	JobID           string `db:"JobID"`
	JobKey          int    `db:"JobKey"`
	Status          int    `db:"Status"`
	Name            string `db:"Name"`
	TriggerType     string `db:"TriggerType"`
	Expression      string `db:"Expression"`
	HttpMethod      string `db:"HttpMethod"`
	HttpTargetUrl   string `db:"HttpTargetUrl"`
	HttpRequestBody string `db:"HttpRequestBody"`
	JsonWebToken    string `db:"JsonWebToken"`
	CreationTime    int64  `db:"CreationTime"`
	UpdateTime      int64  `db:"UpdateTime"`
}

const (
	JobStatusEnable  = 1
	JobStatusDisable = 2
	JobStatusDone    = 3
)
