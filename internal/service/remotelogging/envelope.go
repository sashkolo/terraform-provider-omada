package remotelogging

// Lenient, provider-local decode types for this resource's result payload. The
// {errorCode, msg, result} envelope itself is decoded by internal/envelope,
// which also explains why these types exist: the SDK's strict models reject
// fields the controller returns that they don't know.

// remoteLoggingReadVO is the result of GET /remote-logging.
type remoteLoggingReadVO struct {
	RemoteLog *remoteLogVO `json:"remoteLog"`
}

// remoteLogVO is the remote syslog target. Resource (0 new, 1 from a site
// template, 2 template override) is informational and not modelled.
type remoteLogVO struct {
	Enable        bool    `json:"enable"`
	Host          *string `json:"host"`
	Port          *int32  `json:"port"`
	MoreClientLog *bool   `json:"moreClientLog"`
}
