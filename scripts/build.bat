SET CGO_ENABLED=0
set APPNAME=caddy

SET GOOS=windows
SET GOARCH=amd64
go build -o %APPNAME%_%GOOS%_%GOARCH%.exe

SET GOOS=linux
SET GOARCH=amd64
go build -o %APPNAME%_%GOOS%_%GOARCH%

SET GOOS=linux
SET GOARCH=arm64
go build -o %APPNAME%_%GOOS%_%GOARCH%

SET GOOS=linux
SET GOARCH=mips
go build -o %APPNAME%_%GOOS%_%GOARCH%

