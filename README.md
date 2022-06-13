# search v2

Middleware for [Caddy](https://caddyserver.com).

**search** indexes your static and dynamic documents then serves a HTTP search endpoint.
Folked from https://github.com/blevesearch . Modified to support Caddy V2, with a lot of new features.

* Support Caddy V2
* Use Bleve V2
* Chinese segmentor support
* Static file watcher
* Mime type auto detection 

### Syntax

```
search [directory|regexp] [endpoint "search/"]
```
* **directory** is the path, relative to site root, to a directory (static content)
* **regexp** is the URL [regular expression] of documents that must be indexed (static and dynamic content)
* **endpoint** is the path, relative to site's root url, of the search endpoint

For more options, use the following syntax:

```
search {
    dbname      (default: md5 of root)
    root        (default: .)(required)
    engine      (default: bleve)
    datadir     (default: /tmp/caddyIndex)
    endpoint    (default: /search)
    template    (default: nil)
    numworkers  (default: nuncpus/2)
    expire      (default: 0)
    filewatcher (default: true)
    analyzer    (default: standard)
    maxsize     (default: 50*1024*1024)

    +path       regexp
    -path       regexp
}
```
* **dbname** is the engine for indexing and searching
* **root** is the site root (required) (should be the same for the root directive)
* **engine** is the engine for indexing and searching
* **datadir** is the absolute path to where the indexer should store all data
* **template** is the path to the search's HTML result's template
* **numworkers** is the number of the index workers
* **expire** is the duration (in seconds) for the static files in site root to be rescaned, default 0 meams not to scan the file
* **filewatcher** true to enable filewatcher for the root
* **analyzer** token analyzer for bleve, default is 'standard', use 'sego' for indexing Chinese
* **maxsize** max file size for indexed files
* **+path** include a path to be indexed (can be added multiple times)
* **-path** exclude a path from being index (can be added multiple times)


### Supported Engines

* [BleveSearch v2](http://github.com/blevesearch/bleve)

### Caddyfile Examples
```
localhost:2016 {
	root * /tmp/www
	route {
		search {
			root        "/tmp/www"
			datadir     "./db"
			endpoint    "/search"
			expire      0		
			analyzer    "sego"
			numworker   2
			+path /static/docs/
			-path ^/blog/admin/
			-path robots.txt
		}
		file_server
	}
}
```

## How to build
* Put in under caddy/modules, import `github.com/caddyserver/caddy/v2/modules/caddy-search` in caddy/cmd/caddy/main.go
* Or use xcaddy
