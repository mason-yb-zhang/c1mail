package main

import (
	"fmt"
	"os"

	"c1mail/mail"
)

func main() {
	a, err := mail.LoadAccount(os.Args[1])
	if err != nil {
		fmt.Println("load:", err)
		os.Exit(1)
	}
	c, err := mail.IMAPDial(a.IMAPHost, a.IMAPPort, a.IMAPImplicitTLS(), a.SkipTLS)
	if err != nil {
		fmt.Println("dial:", err)
		os.Exit(1)
	}
	defer c.Close()
	if err := c.Login(a.Email, a.Password); err != nil {
		fmt.Println("login:", err)
		os.Exit(1)
	}
	fmt.Println("login OK")
	n, err := c.Select("INBOX")
	if err != nil {
		fmt.Println("select:", err)
		os.Exit(1)
	}
	fmt.Println("exists", n)
	list, err := c.UIDFetchHeaders(n, 3)
	if err != nil {
		fmt.Println("headers:", err)
		os.Exit(1)
	}
	fmt.Println("got", len(list))
	for _, s := range list {
		fmt.Printf(" uid=%d subj=%.40s\n", s.UID, s.Subject)
	}
	if len(list) == 0 {
		return
	}
	fmt.Println("fetch body uid", list[0].UID)
	raw, err := c.UIDFetchBody(list[0].UID)
	fmt.Println("body bytes", len(raw), "err", err)
	if err != nil {
		return
	}
	m, perr := mail.ParseMessage(list[0].UID, raw)
	fmt.Println("parse err", perr)
	if m != nil {
		fmt.Printf("body preview: %.150q\n", m.Body)
	}
}
