## ipcs

[![GoDoc](https://godoc.org/github.com/hinshun/ipcs?status.svg)](https://godoc.org/github.com/hinshun/ipcs)
[![Build Status](https://travis-ci.org/hinshun/ipcs.svg?branch=master)](https://travis-ci.org/hinshun/ipcs)
[![Go Report Card](https://goreportcard.com/badge/github.com/hinshun/ipcs)](https://goreportcard.com/report/github.com/hinshun/ipcs)

proxy.NewContentStore (content.Store)
	-> content.ContentClient (gRPC client)
		-> content.NewService (gRPC server: plugin.GRPCPlugin, "content")
			-> content.newContentStore (content.Store: plugin.ServicePlugin, services.ContentService)
				-> metadata.NewDB (bolt *metadata.DB: plugin.MetadataPlugin, "bolt")
					-> *LOADED* ipcs.NewContentStore (content.Store: plugin.ContentPlugin, "ipcs")
					-> local.NewStore (content.Store: plugin.ContentPlugin, "content")
					
- (*metadata.DB).Info uses boltdb buckets, never hits wrapped content.Store
- (*namespacedWriter).Writer creates boltdb bucket on commit when writing to content.Store
