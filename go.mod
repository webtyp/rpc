module webtyp.com/rpc

go 1.26.8

require (
	webtyp.com/crud v0.0.2
	webtyp.com/fetch v0.1.29
	webtyp.com/fmt v1.0.0
	webtyp.com/json v0.5.40
	webtyp.com/model v0.2.13
	webtyp.com/router v0.4.17
)

require (
	webtyp.com/escape v0.1.0 // indirect
	webtyp.com/filepath v0.1.0 // indirect
)

replace webtyp.com/model => ../model

replace webtyp.com/router => ../router

replace webtyp.com/crud => ../crud
