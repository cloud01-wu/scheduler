CREATE TABLE IF NOT EXISTS `schedule_jobs` (
  `JobID` varchar(36) NOT NULL COMMENT 'job uuid',
  `JobKey` bigint(20) NOT NULL COMMENT 'job key from quartz',
  `Status` tinyint(4) NOT NULL DEFAULT 0 COMMENT '1:enable,2:disable,3:done',
  `Name` varchar(32) NOT NULL COMMENT 'job name',
  `TriggerType` varchar(8) NOT NULL COMMENT 'job trigger type',
  `Expression` varchar(64) NOT NULL COMMENT 'trigger expression',
  `HttpMethod` varchar(8) NOT NULL COMMENT 'http method',
  `HttpTargetUrl` varchar(320) NOT NULL COMMENT 'http target url',
  `HttpRequestBody` text NOT NULL COMMENT 'http request body',
  `JsonWebToken` text NOT NULL COMMENT 'jwt',
  `CreationTime` bigint(20) NOT NULL COMMENT 'creation time epoch',
  `UpdateTime` bigint(20) NOT NULL COMMENT 'update time epoch'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='schedule jobs table';

ALTER TABLE `schedule_jobs`
  ADD PRIMARY KEY (`JobID`),
  ADD UNIQUE KEY `JOB_KEY` (`JobID`,`JobKey`),
  ADD KEY `CREATION_TIME` (`CreationTime`),
  ADD KEY `UPDATE_TIME` (`UpdateTime`),
  ADD KEY `STATUS` (`Status`);