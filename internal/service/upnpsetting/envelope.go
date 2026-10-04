package upnpsetting

// Lenient, provider-local decode type for this resource's result payload. The
// {errorCode, msg, result} envelope itself is decoded by internal/envelope,
// which also explains why these types exist: the SDK's strict models reject
// fields the controller returns that they don't know.

// upnpReadVO is the result of GET /upnp. The controller leaves networkIds and
// wanPortIds out when none are selected; they read as empty sets.
type upnpReadVO struct {
	Enable                 bool     `json:"enable"`
	NetworkIds             []string `json:"networkIds"`
	WanPortIds             []string `json:"wanPortIds"`
	SupportByDsLiteAndMapE *bool    `json:"supportByDsLiteAndMapE"`
}
