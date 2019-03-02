package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// NewDirectory create directory recursively
func NewDirectory(path string) bool {
	if fileInfo, err := os.Stat(path); err == nil {
		if !fileInfo.IsDir() {
			if err = os.Remove(path); err != nil {
				return false
			}
			if err = os.MkdirAll(path, 0700); err != nil {
				return false
			}
		}
	} else {
		if err = os.MkdirAll(path, 0700); err != nil {
			return false
		}
	}

	return true
}

// IntPow simple int pow
func IntPow(a int32, b int32) int32 {
	if b <= 0 {
		return 1
	}
	return IntPow(a, b-1) * a
}

// IPtoInt32 php compatiable function
func IPtoInt32(address string) int32 {
	addresses := strings.Split(address, ".")
	if len(addresses) != 4 {
		return 0
	}
	var intv int32
	for i := range addresses {
		subIntv, err := strconv.ParseInt(addresses[i], 10, 32)
		if err != nil {
			return 0
		}
		intv += int32(subIntv) * IntPow(int32(2), int32(8*(3-i)))
	}
	return intv
}

// FtpdOption ftpd params struct
type FtpdOption struct {
	Directory         string
	Address           string
	PASVAddressFunc   func() string
	LogFunc           func(client string, msg string)
	AuthFunc          func(client string, username string, password string) bool
	FileTransAuthFunc func(client string, username string, kind int, path string) bool // kind 1 list 2 get 3 store
}

func ftpdSplit(message string) (string, string) {
	command := strings.Split(strings.Trim(message, "\r\n "), " ")
	cmd := command[0]
	args := strings.Join(command[1:len(command)], " ")
	return cmd, args
}

// Ftpd a simple ftp server
func Ftpd(params *FtpdOption) {
	if params.LogFunc == nil {
		fmt.Println("ftpd needs logger")
		return
	}

	if params.FileTransAuthFunc == nil {
		params.FileTransAuthFunc = func(client string, username string, kind int, path string) bool { return true }
	}
	server, err := net.Listen("tcp4", params.Address)
	if err != nil {
		params.LogFunc("", fmt.Sprintf("ftpd can not bind %s", err.Error()))
		return
	}

	defer server.Close()

	params.LogFunc("", fmt.Sprintf("ftpd bind %s", params.Address))
	if params.Directory == "" {
		params.Directory = "."
	}
	params.Directory, err = filepath.Abs(params.Directory)
	if err != nil {
		params.LogFunc("", fmt.Sprintf("ftpd can not get absolute path of `%s`", params.Directory))
		return
	}
	params.Directory = filepath.ToSlash(params.Directory + "/")

	fileInfo, err := os.Stat(params.Directory)
	if err != nil || !fileInfo.IsDir() {
		params.LogFunc("", fmt.Sprintf("ftpd can not use path `%s` as root directory", params.Directory))
		return
	}

	// easy to debug or log
	var reply = func(conn net.Conn, message string) {
		// log output
		//params.LogFunc(conn.RemoteAddr().String(), strings.Trim(message, "\n"))
		conn.Write([]byte(message))
	}

	for {
		// RNFR + RNTO
		lastCmd := ""
		lastcPath := ""
		// REST for RETR
		var fileStart int64

		client, err := server.Accept()
		if err != nil {
			params.LogFunc(client.RemoteAddr().String(), fmt.Sprintf("ftpd can not accept, error: %s", err.Error()))
			continue
		}
		params.LogFunc(client.RemoteAddr().String(), "ftpd accept new client")

		go func(conn net.Conn) {
			defer conn.Close()

			ln, err := net.Listen("tcp4", server.Addr().(*net.TCPAddr).IP.String()+":")
			if err != nil {
				params.LogFunc("", fmt.Sprintf("ftpd can not open pasv data listener %s", server.Addr().(*net.TCPAddr).IP.String()+":"))
				return
			}
			defer ln.Close()

			reply(conn, "220 Ready.\r\n")

			authorized := false
			if params.AuthFunc == nil {
				authorized = true
			}
			transAuthorized := true
			authorizeName := ""
			pwd := "/"

			for {
				message, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil {
					if err == io.EOF {
						params.LogFunc(client.RemoteAddr().String(), "ftpd lost client")
						break
					}
					continue
				}

				// log input
				//params.LogFunc(conn.RemoteAddr().String(), strings.Trim(message, "\n"))

				transAuthorized = true
				cmd, args := ftpdSplit(message)

				if authorizeName == "" {
					if cmd != "USER" {
						reply(conn, "530 Please Login.\r\n")
						continue
					} else {
						authorizeName = args
						if params.AuthFunc == nil {
							reply(conn, "230 Hello, "+authorizeName+".\r\n")
						} else {
							reply(conn, "331 Hello, "+authorizeName+".\r\n")
						}
						continue
					}
				}

				if !authorized && cmd != "PASS" {
					reply(conn, "530 Need authorization first.\r\n")
					continue
				}

				if authorized && (cmd == "USER" || cmd == "PASS") {
					reply(conn, "530 Forbid authorization twice.\r\n")
					break
				}

				// try to get the absolute request path from args (valid but not really exists)
				cPath := args
				if !strings.HasPrefix(cPath, "/") {
					cPath = strings.TrimRight(pwd, "/") + "/" + cPath
				}
				cPath, err = filepath.Abs(strings.TrimRight(params.Directory, "/") + cPath)
				cPath = filepath.ToSlash(cPath)
				if cPath == strings.TrimRight(params.Directory, "/") {
					cPath = params.Directory
				}
				if err != nil || !strings.HasPrefix(cPath, params.Directory) {
					cPath = ""
				}

				switch cmd {
				case "SYST":
					reply(conn, "215 UNIX Type: GO(CROSS-PLATFORM).\r\n")

				case "PASS":
					if params.AuthFunc(client.RemoteAddr().String(), authorizeName, args) {
						authorized = true
						reply(conn, "230 Authorization success.\r\n")
					} else {
						reply(conn, "530 Authorization failure.\r\n")
					}

				case "PASV":
					tcpAddr := ln.Addr().(*net.TCPAddr)
					PUBAddress := conn.LocalAddr().(*net.TCPAddr).IP.String()
					if params.PASVAddressFunc != nil {
						pasvAddr := params.PASVAddressFunc()
						if IPtoInt32(pasvAddr) != 0 {
							PUBAddress = pasvAddr
						} else {
							params.LogFunc("", fmt.Sprintf("invalid ip `%s` from PASVAddressFunc, use local address", pasvAddr))
						}
					}

					address := strings.Replace(PUBAddress, ".", ",", -1) + "," + strconv.Itoa(tcpAddr.Port>>8) + "," + strconv.Itoa(tcpAddr.Port&0xff)
					reply(conn, "227 Entering passive mode ("+address+").\r\n")

				case "CDUP":
					cPath := filepath.Dir(strings.TrimRight(params.Directory, "/") + pwd)
					cPath = filepath.ToSlash(cPath)
					if cPath+"/" == params.Directory {
						cPath = params.Directory
					}
					if !strings.HasPrefix(cPath, params.Directory) {
						reply(conn, "550 Can not change into parent directory.\r\n")
						continue
					}
					pwd = "/" + strings.TrimPrefix(cPath, params.Directory)
					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 1, pwd)
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					reply(conn, "250 Directory successfully changed.\r\n")

				case "CWD":
					if cPath == "" {
						reply(conn, fmt.Sprintf("550 Can not change into directory `%s`.\r\n", args))
						continue
					}

					pwd = "/" + strings.TrimPrefix(cPath, params.Directory)
					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 1, pwd)
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					// winscp will not send MKD
					os.MkdirAll(cPath, 0700)
					reply(conn, "250 Directory successfully changed.\r\n")

				case "PWD":
					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 1, pwd)
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					reply(conn, "257 \""+pwd+"\"\n")

				case "LIST":
					reply(conn, "150 Opening passive mode data connection.\r\n")

					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 1, pwd)
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					dataConn, err := ln.Accept()
					if err != nil {
						reply(conn, "425 Failed to open data connection.\r\n")
						continue
					}

					files, err := ioutil.ReadDir(strings.TrimRight(params.Directory, "/") + pwd)
					if err != nil {
						reply(conn, "451 Failed to retrieve directory listing.\r\n")
						dataConn.Close()
						continue
					}
					for _, file := range files {
						modTime := file.ModTime().Add(time.Duration(-8) * time.Hour)
						if file.IsDir() {
							reply(dataConn, fmt.Sprintf("drwx------   3 user group %12d %s %2d %s %s\r\n", file.Size(), modTime.Format("Jan"), modTime.Day(), modTime.Format("15:04"), file.Name()))
						} else {
							reply(dataConn, fmt.Sprintf("-rwx------   1 user group %12d %s %2d %s %s\r\n", file.Size(), modTime.Format("Jan"), modTime.Day(), modTime.Format("15:04"), file.Name()))
						}
					}
					dataConn.Close()
					reply(conn, "226 Transfer completed.\r\n")

				case "TYPE":
					switch args[0] {
					case 'A':
						// IGNORE
						reply(conn, "200 OK.\r\n")
					case 'I':
						reply(conn, "200 OK.\r\n")
					default:
						reply(conn, "504 Command not implemented for that parameter.\r\n")
					}

				case "SIZE":
					fallthrough
				case "MDTM":
					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 1, strings.TrimPrefix(cPath, params.Directory))
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					if cPath == "" {
						reply(conn, "550 "+args+": No such file or directory.\r\n")
						continue
					}

					fileInfo, err := os.Stat(cPath)
					if err != nil {
						reply(conn, "550 "+args+": No such file or directory.\r\n")
						continue
					} else {
						if cmd == "MDTM" {
							reply(conn, fmt.Sprintf("213 %s\n", fileInfo.ModTime().Format("20060102150405.999")))
						} else {
							reply(conn, fmt.Sprintf("213 %d\n", fileInfo.Size()))
						}
					}

				case "MKD":
					if cPath == "" {
						reply(conn, "550 Invalid directory name.\r\n")
						continue
					}

					if _, err := os.Stat(cPath); err == nil {
						reply(conn, "550 Path exists.\r\n")
						continue
					}

					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 3, strings.TrimPrefix(cPath, params.Directory))
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					err = os.Mkdir(cPath, 0700)
					if err != nil {
						reply(conn, "550 Invalid directory name.\r\n")
						continue
					}

					reply(conn, "257 \""+strings.TrimPrefix(cPath, params.Directory)+"\"\n")

				case "RMD":
					if cPath == "" {
						reply(conn, "550 Directory not found.\r\n")
						continue
					}

					files, err := ioutil.ReadDir(cPath)
					if err != nil || len(files) != 0 {
						reply(conn, "550 Directory not empty.\r\n")
						continue
					}

					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 3, strings.TrimPrefix(cPath, params.Directory))
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					err = os.Remove(cPath)
					if err != nil {
						reply(conn, "550 Directory not found.\r\n")
						continue
					}

					reply(conn, "250 OK.\r\n")

				case "DELE":
					if cPath == "" {
						reply(conn, "550 File not found.\r\n")
						continue
					}

					params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 3, strings.TrimPrefix(cPath, params.Directory))
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					err = os.Remove(cPath)
					if err != nil {
						reply(conn, "550 File not found.\r\n")
						continue
					}

					reply(conn, "250 OK.\r\n")

				case "ALLO":
					reply(conn, "202 ALLO is obsolete.\r\n")

				case "REST":
					fileStart, err = strconv.ParseInt(args, 10, 64)
					if err != nil {
						fileStart = 0
						reply(conn, "550 Error restart point.\r\n")
						continue
					}
					reply(conn, fmt.Sprintf("350 Restarting at %d. Send STORE or RETRIEVE.\r\n", fileStart))

				case "RETR":
					fallthrough
				case "STOR":
					fallthrough
				case "APPE":
					if cPath == "" {
						reply(conn, "550 File not found.\r\n")
						continue
					}

					mode := os.O_RDONLY
					if cmd == "STOR" || cmd == "APPE" {
						mode = os.O_RDWR | os.O_CREATE
						if cmd == "APPE" {
							mode = os.O_RDWR | os.O_CREATE | os.O_APPEND
						}
						transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 3, strings.TrimPrefix(cPath, params.Directory))
					} else {
						transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 2, strings.TrimPrefix(cPath, params.Directory))
					}
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					if _, err := os.Stat(cPath); err != nil {
						NewDirectory(path.Dir(cPath))
					}

					f, err := os.OpenFile(cPath, mode, 0600)
					if err != nil {
						reply(conn, "550 Error when opening file.\r\n")
						continue
					}

					reply(conn, "150 Opening passive mode data connection.\r\n")
					dataConn, err := ln.Accept()
					if err != nil {
						reply(conn, "425 Failed to open data connection.\r\n")
						f.Close()
						continue
					}

					buf := make([]byte, 1024)
					if cmd == "RETR" {
						if fileStart != 0 {
							f.Seek(fileStart, os.SEEK_SET)
							fileStart = 0
						}
						_, err = io.CopyBuffer(dataConn, f, buf)
					} else {
						_, err = io.CopyBuffer(f, dataConn, buf)
					}
					if err != nil {
						reply(conn, "550 Transfer failed.\r\n")
					} else {
						reply(conn, "226 Transfer completed.\r\n")
					}

					f.Close()
					dataConn.Close()

				case "RNFR":
					if cPath == "" {
						reply(conn, "550 Not found.\r\n")
						continue
					}
					reply(conn, "350 Rename start.\r\n")

				case "RNTO":
					if lastCmd != "RNFR" {
						reply(conn, "550 Choose file first.\r\n")
						continue
					}

					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 3, strings.TrimPrefix(lastcPath, params.Directory))
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					transAuthorized = params.FileTransAuthFunc(client.RemoteAddr().String(), authorizeName, 3, strings.TrimPrefix(cPath, params.Directory))
					if !transAuthorized {
						reply(conn, "530 Access denied.\r\n")
						continue
					}

					NewDirectory(path.Dir(cPath))
					err := os.Rename(lastcPath, cPath)
					if err != nil {
						reply(conn, "550 Rrenanme error.\r\n")
						continue
					}
					reply(conn, "250 Rename completed.\r\n")

				case "QUIT":
					reply(conn, "221 Bye.\r\n")
					break

				default:
					reply(conn, "502 Command not implemented.\r\n")

				}

				// RNFR + RNTO
				lastCmd = cmd
				lastcPath = cPath
			}
		}(client)
	}
}

func main() {
	wait := make(chan bool, 1)
	go func() {
		Ftpd(&FtpdOption{
			Address: "0.0.0.0:2221",
			LogFunc: func(client string, msg string) {
			},
			AuthFunc: func(client string, username string, password string) bool {
				fmt.Printf("username: %s, password: %s\n", username, password)
				if username == "ftp" && password == "ftp" {
					return true
				}

				return false
			},
			FileTransAuthFunc: func(client string, username string, kind int, path string) bool {
				allow := "allow"
				operates := []string{"", "list", "get", "store"}
				if kind == 3 {
					//allow = "disallow"
				}
				fmt.Printf("%s user `%s` %s path `%s`\n", allow, username, operates[kind], path)
				return allow == "allow"
			},
		})
		wait <- true
	}()
	<-wait
}
