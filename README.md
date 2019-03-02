# go-ftpd-func
It's hard to find a usable ftp server, at last got a demo `https://github.com/fabiano/ftpd/blob/master/ftpd.go`

# client see https://github.com/di3663/go-ftp-func

## 1.cross-platform

support filezilla\/winscp client

```
modTime := file.ModTime().Add(time.Duration(-8) * time.Hour) // change server timezone

// client timezone
// 1.filezilla choose timezone
// 2.winscp close mlsd and choose timezone
```

support php ftp commands
```
ftp_connect
ftp_systype
ftp_login
ftp_pasv
ftp_size
ftp_mdtm
ftp_mkdir
ftp_rmdir
ftp_cdup
ftp_chdir
ftp_pwd
ftp_put
ftp_append
ftp_rename
ftp_raw($ftpConn, 'REST <point>');
ftp_get
ftp_delete
ftp_rawlist
ftp_close
```

## 2.complete commands

support commands
```
SYST
USER
PASS
PASV
CDUP
CWD
PWD
LIST
TYPE
SIZE
MDTM
MKD
RMD
DELE
ALLO
REST
RETR
STOR
APPE
RNFR
RNTO
QUIT
```

## 3.support file trans authorization
```
username: ftp, password: ftp
allow user `ftp` list path `/`
allow user `ftp` get path `main.go`
disallow user `ftp` store path `20171225.ppk`
```

## 4.android demo apk
https://github.com/di3663/go-ftpd-func/raw/master/FTPD.apk

0.0.0.0:2221
ftp/ftp
