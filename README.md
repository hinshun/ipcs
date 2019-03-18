ipcs
---

proxy.NewContentStore (content.Store)
	-> content.ContentClient (gRPC client)
		-> content.NewService (gRPC server: plugin.GRPCPlugin, "content")
			-> content.newContentStore (content.Store: plugin.ServicePlugin, services.ContentService)
				-> metadata.NewDB (bolt *metadata.DB: plugin.MetadataPlugin, "bolt")
					-> *LOADED* ipcs.NewContentStore (content.Store: plugin.ContentPlugin, "ipcs")
					-> local.NewStore (content.Store: plugin.ContentPlugin, "content")
					
- (*metadata.DB).Info uses boltdb buckets, never hits wrapped content.Store
- (*namespacedWriter).Writer creates boltdb bucket on commit when writing to content.Store
