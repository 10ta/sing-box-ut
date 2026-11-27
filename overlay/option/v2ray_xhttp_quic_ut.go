package option

// ut: V2RayXHTTPQUICParamsOptions is the XHTTP transport's quic_params, the
// shared QUICParamsOptions (quic_params_ut.go).
//
//	"transport": {
//	  "type": "xhttp", ...,
//	  "quic_params": { "congestion": "brutal", "up_mbps": 60 }
//	}
type V2RayXHTTPQUICParamsOptions = QUICParamsOptions
