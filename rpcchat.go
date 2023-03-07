package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	MsgRegister = iota
	MsgList
	MsgCheckMessages
	MsgTell
	MsgSay
	MsgQuit
	MsgShutdown
)

var mutex sync.Mutex
var messages map[string][]string
var shutdown chan struct{}

func server(listenAddress string) {
	shutdown = make(chan struct{})
	messages = make(map[string][]string)

	// set up network listen and accept loop here
	// to receive and dispatch RPC requests
	// ...

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("Failed to listen on address %s: %s", listenAddress, err)
	}
	defer listener.Close()

	log.Printf("Server listening on %s", listenAddress)

	// start the RPC server loop
	for {
		conn, err := listener.Accept()
		if err == nil {
			go connection(conn)
		}
		//go rpc.ServeConn(conn)
	}

	// wait for a shutdown request
	<-shutdown
	time.Sleep(100 * time.Millisecond)
}

func connection(conn net.Conn) {
	defer conn.Close()
	msgs, err := ReadUint16(conn)
	if err != nil {
		log.Printf("error: %v", err)
		return
	}
	switch msgs {
	case MsgRegister:
		user, err := ReadString(conn)
		Err := ""
		if err != nil {
			log.Printf("register message decoding error, %v", err)
			return
		}
		if err := Register(user); err != nil {
			log.Printf("error with register message, %v", err)
			Err = "Register error"
		}
		WriteString(conn, Err)
	case MsgList:
		users := List()
		err = WriteStringSlice(conn, users)
		if err != nil {
			log.Printf("list response encoding error, %v", err)
		}

		WriteString(conn, "")

	case MsgCheckMessages:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("checkmessages decoding error, %v", err)
			return
		}
		messages := CheckMessages(user)
		err = WriteStringSlice(conn, messages)
		if err != nil {
			log.Printf("checkmessages encoding error, %v", err)
		}
		WriteString(conn, "")
	case MsgTell:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("tell message decoding error, %v", err)
			return
		}
		target, err := ReadString(conn)
		if err != nil {
			log.Printf("tell message encoding error, %v", err)
			return
		}
		message, err := ReadString(conn)
		if err != nil {
			log.Printf("tell message decoding error, %v", err)
			return
		}
		err = WriteString(conn, "")
		if err != nil {
			log.Printf("list response encoding error, %v", err)
		}
		Tell(user, target, message)
		WriteString(conn, "")
	case MsgSay:

		user, err := ReadString(conn)
		if err != nil {
			log.Printf("say message decoding error, %v", err)
			return
		}
		message, err := ReadString(conn)
		if err != nil {
			log.Printf("say message decoding error, %v", err)
			return
		}
		Say(user, message)
		WriteString(conn, "")
	case MsgQuit:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("quit message decoding error, %v", err)
			return
		}
		Quit(user)
		WriteString(conn, "")
	case MsgShutdown:
		WriteString(conn, "")
		Shutdown()
	default:
		log.Printf("unknown message type: %d", msgs)
	}

}

func Register(user string) error {
	if len(user) < 1 || len(user) > 20 {
		return fmt.Errorf("Register: user must be between 1 and 20 letters")
	}
	for _, r := range user {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("Register: user must only contain letters and digits")
		}
	}
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("*** %s has logged in", user)
	log.Printf(msg)
	for target, queue := range messages {
		messages[target] = append(queue, msg)
	}
	messages[user] = nil

	return nil
}

func List() []string {
	mutex.Lock()
	defer mutex.Unlock()

	var users []string
	for target := range messages {
		users = append(users, target)
	}
	sort.Strings(users)

	return users
}

func CheckMessages(user string) []string {
	mutex.Lock()
	defer mutex.Unlock()

	if queue, present := messages[user]; present {
		messages[user] = nil
		return queue
	} else {
		return []string{"*** You are not logged in, " + user}
	}
}

func Tell(user, target, message string) {
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("%s tells you %s", user, message)
	if queue, present := messages[target]; present {
		messages[target] = append(queue, msg)
	} else if queue, present := messages[user]; present {
		messages[user] = append(queue, "*** No such user: "+target)
	}
}

func Say(user, message string) {
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("%s says %s", user, message)
	for target, queue := range messages {
		messages[target] = append(queue, msg)
	}
}

func Quit(user string) {
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("*** %s has logged out", user)
	log.Print(msg)
	for target, queue := range messages {
		messages[target] = append(queue, msg)
	}
	delete(messages, user)
}

func Shutdown() {
	shutdown <- struct{}{}
}

func Help() {
	log.Print("\n tell: messages user directly\n" +
		"say: says message to all users\n" +
		"list: shows list of users\n" +
		"quit: quit program\n" +
		"shutdown: Shutdown server\n")

}

func client(serverAddress, user string) {
	quit := false

	err := RegisterRPC(serverAddress, user)
	if err != nil {
		log.Fatal(err)
	}

	/*for {
		messages, err := CheckMessagesRPC(serverAddress, user)
		if err != nil {
			log.Fatal(err)
		}
		for _, message := range messages {
			log.Print(message)
		}
		time.Sleep(time.Second)
	}*/

	go wait(serverAddress, user)

	for !quit {
		fmt.Print("> ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		err := scanner.Err()
		if err != nil {
			log.Fatal(err)
		}
		s := strings.Split(scanner.Text(), " ")
		switch s[0] {
		case "list":
			commands, err := ListRPC(serverAddress)
			if err != nil {
				log.Fatal("quitting program: command not sent")
			}
			for _, command := range commands {
				log.Print(command)
			}
		case "tell":
			origMsg := ""
			for i, word := range s {
				if i >= 2 {
					origMsg += word
					if i != len(s)-1 {
						origMsg += " "
					}
				}
			}
			err := TellRPC(serverAddress, user, s[1], origMsg)
			if err != nil {
				log.Fatal("quitting program: command not sent")
			}
		case "say":

			origMsg := ""
			for i, word := range s {
				if i >= 1 {
					origMsg += word
					if i != len(s)-1 {
						origMsg += " "
					}
				}
			}
			err := SayRPC(serverAddress, user, origMsg)
			if err != nil {
				log.Fatal("quitting program: command not sent")
			}
		case "quit":
			err := QuitRPC(serverAddress, user)
			if err != nil {
				log.Fatal("quitting program: command not sent")
			}
			quit = true
		case "shutdown":
			err := ShutdownRPC(serverAddress)
			if err != nil {
				log.Fatal("quitting program: command not sent")
			}
			quit = true
		case "help":
			Help()
		case "":
		default:
			log.Print("unrecognized command")

		}
	}
}

func wait(server, user string) {
	for {
		messages, err := CheckMessagesRPC(server, user)
		if err != nil {
			log.Fatal(err)
		}
		for _, message := range messages {
			log.Print(message)
		}
		time.Sleep(time.Second)
	}
}

func RegisterRPC(server, user string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()

	// calling register on server
	err = WriteUint16(conn, MsgRegister)
	if err != nil {
		return err
	}

	err = WriteString(conn, user)
	if err != nil {
		return err
	}

	errString, err := ReadString(conn)
	if err != nil {
		return err
	}
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func CheckMessagesRPC(server string, user string) ([]string, error) {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return make([]string, 0), err
	}
	defer conn.Close()

	err = WriteUint16(conn, MsgCheckMessages)
	if err != nil {
		return make([]string, 0), err
	}

	err = WriteString(conn, user)
	if err != nil {
		return make([]string, 0), err
	}

	message, err := ReadStringSlice(conn)
	if err != nil {
		return make([]string, 0), err
	}

	errString, err := ReadString(conn)
	if err != nil {
		return make([]string, 0), err
	}
	if errString != "" {
		log.Fatal(errString)
	}
	return message, err
}

func ListRPC(server string) ([]string, error) {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return make([]string, 0), err
	}
	defer conn.Close()

	err = WriteUint16(conn, MsgList)
	if err != nil {
		return make([]string, 0), err
	}

	list, err := ReadStringSlice(conn)
	if err != nil {
		return make([]string, 0), err
	}

	errString, err := ReadString(conn)
	if err != nil {
		return make([]string, 0), err
	}
	if errString != "" {
		log.Fatal(errString)
	}
	return list, err

}

func TellRPC(server, user, target, message string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = WriteUint16(conn, MsgTell)
	if err != nil {
		return err
	}

	err = WriteString(conn, user)
	if err != nil {
		return err
	}
	err = WriteString(conn, target)
	if err != nil {
		return err
	}
	err = WriteString(conn, message)
	if err != nil {
		return err
	}

	errString, err := ReadString(conn)
	if err != nil {
		return err
	}
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func SayRPC(server, user, message string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = WriteUint16(conn, MsgSay)
	if err != nil {
		return err
	}

	err = WriteString(conn, user)
	if err != nil {
		return err
	}
	err = WriteString(conn, message)
	if err != nil {
		return err
	}

	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func QuitRPC(server, user string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = WriteUint16(conn, MsgQuit)
	if err != nil {
		return err
	}

	err = WriteString(conn, user)
	if err != nil {
		return err
	}

	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func ReadUint16(r io.Reader) (uint16, error) {
	buf := make([]byte, 2)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	value := uint16(buf[0])<<8 | uint16(buf[1])
	return value, nil
}
func WriteUint16(conn io.Writer, value uint16) error {
	raw := []byte{
		byte((value >> 8) & 0xff),
		byte((value >> 0) & 0xff),
	}
	_, err := conn.Write(raw)
	return err
}
func WriteString(conn io.Writer, value string) error {
	WriteUint16(conn, uint16(len(value)))
	_, err := io.WriteString(conn, value)
	return err
}

func WriteStringSlice(conn io.Writer, value []string) error {
	err := WriteUint16(conn, uint16(len(value)))
	for _, x := range value {
		err = WriteString(conn, x)
	}
	return err
}
func ReadString(r io.Reader) (string, error) {
	strLen, err := ReadUint16(r)
	if err != nil {
		return "", err
	}
	strBuf := make([]byte, strLen)
	_, err = io.ReadFull(r, strBuf)
	if err != nil {
		return "", err
	}
	return string(strBuf), nil
}
func ReadStringSlice(r io.Reader) ([]string, error) {
	strsLen, err := ReadUint16(r)
	if err != nil {
		return nil, err
	}
	strs := make([]string, strsLen)
	for i := uint16(0); i < strsLen; i++ {
		str, err := ReadString(r)
		if err != nil {
			return nil, err
		}
		strs[i] = str
	}
	return strs, nil
}

func ShutdownRPC(server string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = WriteUint16(conn, MsgShutdown)
	if err != nil {
		return err
	}

	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func main() {
	log.SetFlags(log.Ltime)

	var listenAddress string
	var serverAddress string
	var username string

	switch len(os.Args) {
	case 2:
		listenAddress = net.JoinHostPort("", os.Args[1])
	case 3:
		serverAddress = os.Args[1]
		if strings.HasPrefix(serverAddress, ":") {
			serverAddress = "localhost" + serverAddress
		}
		username = strings.TrimSpace(os.Args[2])
		if username == "" {
			log.Fatal("empty user name")
		}
	default:
		log.Fatalf("Usage: %s <port>   OR   %s <server> <user>",
			os.Args[0], os.Args[0])
	}

	if len(listenAddress) > 0 {
		server(listenAddress)
	} else {
		client(serverAddress, username)
	}
}
