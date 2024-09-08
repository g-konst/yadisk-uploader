Simple uploader to Yandex Disk using [yadisk-go](https://github.com/g-konst/yadisk-go) client.

## Build

```shell
go build -o yadisk-uploader main.go
```


## Usage

```shell
yadisk-uploader -i ./my-local-folder -o /some-folder-on-disk -w 4

Usage of yadisk-uploader:
  -i string
        Path on local (default ".")
  -o string
        Path on Yandex Disk (default "disk:/")
  -r int
        Max attempt count (default 1)
  -token string
        Yandex Disk OAuth token
  -w int
        Workers count (default 1)
```
