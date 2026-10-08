package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

const defaultServer = "https://tryhisar.com"

func deviceInfo() proto.DeviceInfo {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	if len(host) > 100 {
		host = host[:100]
	}
	return proto.DeviceInfo{Name: host, Platform: runtime.GOOS + "/cli"}
}

func authedClient(st *state) *client.Client {
	c := client.New(st.Server)
	c.Token = st.Token
	return c
}

// unlock prompts for the master password and decrypts the keyset.
func unlock(ctx context.Context) (*state, *session.Session, error) {
	st, err := loadState()
	if err != nil {
		return nil, nil, err
	}
	if err := st.requireLogin(); err != nil {
		return nil, nil, err
	}
	sk, err := loadSecretKey(st.AccountID)
	if err != nil {
		return nil, nil, err
	}
	pw, err := promptSecret("Master password")
	if err != nil {
		return nil, nil, err
	}
	defer clear(pw)
	s, err := session.Unlock(ctx, authedClient(st), pw, sk)
	if client.IsStatus(err, 401) {
		return nil, nil, errors.New("session expired or device revoked; run `hisar login`")
	}
	if err != nil {
		return nil, nil, err
	}
	s.Seqs = st
	return st, s, nil
}

func finish(st *state, s *session.Session, err error) error {
	if s != nil {
		s.Lock()
	}
	if serr := st.save(); err == nil {
		err = serr
	}
	return err
}

func cmdSignup(ctx context.Context, fs *flag.FlagSet, args []string) error {
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	server, token, err := parseInvite(pos[0])
	if err != nil {
		return err
	}

	email, err := promptRequired("Email", "")
	if err != nil {
		return err
	}
	name, err := promptRequired("Your name", "")
	if err != nil {
		return err
	}
	pw, err := promptNewPassword()
	if err != nil {
		return err
	}
	defer clear(pw)

	fmt.Fprintln(os.Stderr, "Creating account...")
	s, sk, err := session.Signup(ctx, client.New(server), token, email, name, pw, deviceInfo())
	if err != nil {
		return err
	}
	defer s.Lock()

	st, err := loadState()
	if err != nil {
		return err
	}
	*st = state{path: st.path, Server: server, Email: strings.ToLower(email), AccountID: s.AccountID, Token: s.Client.Token, Pins: map[string][]byte{}, Seqs: map[string]int64{}}
	if err := st.save(); err != nil {
		return err
	}
	if err := storeSecretKey(sk); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save Secret Key to keychain: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, `
Account created. This is your Secret Key:

    %s

Write it down and keep it with your master password somewhere safe.
You need both to sign in on a new device; nobody can recover it for you.
`, sk.Encode())
	return nil
}

func parseInvite(link string) (server, token string, err error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", "", errors.New("invalid invite link")
	}
	token, ok := strings.CutPrefix(u.Path, "/invite/")
	if !ok || token == "" || strings.Contains(token, "/") {
		return "", "", errors.New("invalid invite link")
	}
	return u.Scheme + "://" + u.Host, token, nil
}

func cmdLogin(ctx context.Context, fs *flag.FlagSet, args []string) error {
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	server, err := promptRequired("Server", firstNonEmpty(st.Server, defaultServer))
	if err != nil {
		return err
	}
	email, err := promptRequired("Email", st.Email)
	if err != nil {
		return err
	}
	var sk crypt.SecretKey
	if st.AccountID != "" && strings.EqualFold(email, st.Email) {
		sk, err = loadSecretKey(st.AccountID)
	}
	if st.AccountID == "" || err != nil || !strings.EqualFold(email, st.Email) {
		raw, err := promptSecret("Secret Key")
		if err != nil {
			return err
		}
		if sk, err = crypt.ParseSecretKey(string(raw)); err != nil {
			return err
		}
	}
	pw, err := promptSecret("Master password")
	if err != nil {
		return err
	}
	defer clear(pw)

	s, err := session.Login(ctx, client.New(server), email, pw, sk, deviceInfo())
	if err != nil {
		return err
	}
	defer s.Lock()

	if st.AccountID != s.AccountID {
		*st = state{path: st.path, Pins: map[string][]byte{}, Seqs: map[string]int64{}}
	}
	st.Server, st.Email, st.AccountID, st.Token = server, s.Email, s.AccountID, s.Client.Token
	if err := st.save(); err != nil {
		return err
	}
	if err := storeSecretKey(sk); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save Secret Key to keychain: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "Signed in as %s (%s).\n", s.Name, s.Email)
	return nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func cmdLogout(ctx context.Context, fs *flag.FlagSet, args []string) error {
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	if err := st.requireLogin(); err != nil {
		return err
	}
	if err := authedClient(st).Logout(ctx); err != nil && !client.IsStatus(err, 401) {
		return err
	}
	st.Token = ""
	return st.save()
}

func cmdWhoami(ctx context.Context, fs *flag.FlagSet, args []string) error {
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	if err := st.requireLogin(); err != nil {
		return err
	}
	a, err := authedClient(st).Account(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s <%s>\naccount: %s\nrole:    %s\nserver:  %s\n", a.Name, a.Email, a.AccountID, a.Role, st.Server)
	return nil
}

func cmdVaults(ctx context.Context, fs *flag.FlagSet, args []string) error {
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, s, err := unlock(ctx)
	if err != nil {
		return err
	}
	vs, err := s.Vaults(ctx)
	if err == nil {
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tROLE\tID")
		for _, v := range vs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", v.Name, v.Type, v.Role, v.ID)
		}
		w.Flush()
	}
	return finish(st, s, err)
}

func cmdVaultCreate(ctx context.Context, fs *flag.FlagSet, args []string) error {
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	st, s, err := unlock(ctx)
	if err != nil {
		return err
	}
	v, err := s.CreateVault(ctx, pos[0], proto.VaultShared)
	if err == nil {
		fmt.Fprintf(os.Stderr, "Created vault %q. Share it with `hisar share %q <email>`.\n", v.Name, v.Name)
	}
	return finish(st, s, err)
}

func vaultFlag(fs *flag.FlagSet) *string {
	return fs.String("v", "Personal", "vault name or ID")
}

func cmdList(ctx context.Context, fs *flag.FlagSet, args []string) error {
	vault := vaultFlag(fs)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, s, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		v, err := s.FindVault(ctx, *vault)
		if err != nil {
			return err
		}
		items, err := s.Items(ctx, v)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TITLE\tUSERNAME\tURL\tID")
		for _, it := range items {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", it.Overview.Title, it.Overview.Username, strings.Join(it.Overview.URLs, " "), it.ID)
		}
		return w.Flush()
	}()
	return finish(st, s, err)
}

func findItem(items []item.Item, query string) (*item.Item, error) {
	var matches []*item.Item
	for i := range items {
		it := &items[i]
		if it.ID == query || strings.EqualFold(it.Overview.Title, query) {
			return it, nil
		}
		if strings.Contains(strings.ToLower(it.Overview.Title), strings.ToLower(query)) {
			matches = append(matches, it)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no item matches %q", query)
	case 1:
		return matches[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d items; use the exact title or ID:", query, len(matches))
	for _, m := range matches {
		fmt.Fprintf(&b, "\n  %s  %s", m.ID, m.Overview.Title)
	}
	return nil, errors.New(b.String())
}

// withItem unlocks, finds the vault and item, and runs fn.
func withItem(ctx context.Context, vaultName, query string, fn func(*session.Session, *session.Vault, *item.Item) error) error {
	st, s, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		v, err := s.FindVault(ctx, vaultName)
		if err != nil {
			return err
		}
		items, err := s.Items(ctx, v)
		if err != nil {
			return err
		}
		it, err := findItem(items, query)
		if err != nil {
			return err
		}
		return fn(s, v, it)
	}()
	return finish(st, s, err)
}

func cmdGet(ctx context.Context, fs *flag.FlagSet, args []string) error {
	vault := vaultFlag(fs)
	field := fs.String("f", "", "print only this field (title, username, url, password, notes, or a custom field name)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	return withItem(ctx, *vault, pos[0], func(_ *session.Session, _ *session.Vault, it *item.Item) error {
		if *field != "" {
			v, ok := fieldValue(it, *field)
			if !ok {
				return fmt.Errorf("item has no field %q", *field)
			}
			fmt.Println(v)
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "title:\t%s\n", it.Overview.Title)
		if it.Overview.Username != "" {
			fmt.Fprintf(w, "username:\t%s\n", it.Overview.Username)
		}
		for _, u := range it.Overview.URLs {
			fmt.Fprintf(w, "url:\t%s\n", u)
		}
		if it.Details.Password != "" {
			fmt.Fprintf(w, "password:\t%s\n", it.Details.Password)
		}
		for _, f := range it.Details.Fields {
			fmt.Fprintf(w, "%s:\t%s\n", f.Name, f.Value)
		}
		if it.Details.Notes != "" {
			fmt.Fprintf(w, "notes:\t%s\n", it.Details.Notes)
		}
		return w.Flush()
	})
}

func fieldValue(it *item.Item, name string) (string, bool) {
	switch strings.ToLower(name) {
	case "title":
		return it.Overview.Title, true
	case "username":
		return it.Overview.Username, true
	case "url":
		if len(it.Overview.URLs) == 0 {
			return "", false
		}
		return it.Overview.URLs[0], true
	case "password":
		return it.Details.Password, true
	case "notes":
		return it.Details.Notes, true
	}
	for _, f := range it.Details.Fields {
		if strings.EqualFold(f.Name, name) {
			return f.Value, true
		}
	}
	return "", false
}

func promptItem(it *item.Item) error {
	var err error
	if it.Overview.Title, err = promptRequired("Title", it.Overview.Title); err != nil {
		return err
	}
	if it.Overview.Username, err = prompt("Username", it.Overview.Username); err != nil {
		return err
	}
	u, err := prompt("URL", strings.Join(it.Overview.URLs, " "))
	if err != nil {
		return err
	}
	it.Overview.URLs = strings.Fields(u)

	label := "Password (empty to generate)"
	if it.Details.Password != "" {
		label = "Password (empty to keep, 'gen' to generate)"
	}
	pw, err := promptSecret(label)
	if err != nil {
		return err
	}
	switch {
	case string(pw) == "gen" || (len(pw) == 0 && it.Details.Password == ""):
		if it.Details.Password, err = item.Generate(item.DefaultGenOptions); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Generated a new password.")
	case len(pw) > 0:
		it.Details.Password = string(pw)
	}
	it.Details.Notes, err = prompt("Notes", it.Details.Notes)
	return err
}

func cmdAdd(ctx context.Context, fs *flag.FlagSet, args []string) error {
	vault := vaultFlag(fs)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, s, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		v, err := s.FindVault(ctx, *vault)
		if err != nil {
			return err
		}
		it := &item.Item{}
		if err := promptItem(it); err != nil {
			return err
		}
		if _, err := s.SaveItem(ctx, v, it); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved %q to %s.\n", it.Overview.Title, v.Name)
		return nil
	}()
	return finish(st, s, err)
}

func cmdEdit(ctx context.Context, fs *flag.FlagSet, args []string) error {
	vault := vaultFlag(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	return withItem(ctx, *vault, pos[0], func(s *session.Session, v *session.Vault, it *item.Item) error {
		if err := promptItem(it); err != nil {
			return err
		}
		if _, err := s.SaveItem(ctx, v, it); errors.Is(err, session.ErrConflict) {
			return errors.New("item was changed on another device meanwhile; run edit again")
		} else if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved %q.\n", it.Overview.Title)
		return nil
	})
}

func cmdRemove(ctx context.Context, fs *flag.FlagSet, args []string) error {
	vault := vaultFlag(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	return withItem(ctx, *vault, pos[0], func(s *session.Session, v *session.Vault, it *item.Item) error {
		ok, err := confirm(fmt.Sprintf("Delete %q from %s?", it.Overview.Title, v.Name))
		if err != nil || !ok {
			return err
		}
		if _, err := s.DeleteItem(ctx, v, it); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Deleted %q.\n", it.Overview.Title)
		return nil
	})
}

func cmdGenerate(_ context.Context, fs *flag.FlagSet, args []string) error {
	n := fs.Int("n", item.DefaultGenOptions.Length, "length")
	noSymbols := fs.Bool("no-symbols", false, "letters and digits only")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	pw, err := item.Generate(item.GenOptions{Length: *n, Digits: true, Symbols: !*noSymbols})
	if err != nil {
		return err
	}
	fmt.Println(pw)
	return nil
}

func cmdMembers(ctx context.Context, fs *flag.FlagSet, args []string) error {
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	if err := st.requireLogin(); err != nil {
		return err
	}
	ms, err := authedClient(st).Members(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tEMAIL\tROLE\tACCOUNT")
	for _, m := range ms {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.Name, m.Email, m.Role, m.AccountID)
	}
	return w.Flush()
}

func cmdShare(ctx context.Context, fs *flag.FlagSet, args []string) error {
	role := fs.String("r", proto.VaultEditor, "owner, editor or reader")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return errUsage
	}
	st, s, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		v, err := s.FindVault(ctx, pos[0])
		if err != nil {
			return err
		}
		if v.Type == proto.VaultPersonal {
			return errors.New("personal vaults cannot be shared; create one with `hisar vault-create`")
		}
		m, err := s.VerifiedMember(ctx, pos[1])
		if err != nil {
			return err
		}
		if pinned, ok := st.Pins[m.AccountID]; ok && !bytes.Equal(pinned, m.PublicKeys.Sign) {
			return fmt.Errorf("the signing key of %s changed since you last shared with them; not sharing", m.Email)
		}
		if err := s.Share(ctx, v, m, *role); err != nil {
			return err
		}
		st.Pins[m.AccountID] = m.PublicKeys.Sign
		fmt.Fprintf(os.Stderr, "Shared %q with %s as %s.\n", v.Name, m.Email, *role)
		return nil
	}()
	return finish(st, s, err)
}

func cmdInvite(ctx context.Context, fs *flag.FlagSet, args []string) error {
	role := fs.String("r", proto.RoleMember, "admin or member")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	if err := st.requireLogin(); err != nil {
		return err
	}
	var email string
	if len(pos) == 1 {
		email = pos[0]
	} else if email, err = promptRequired("Email", ""); err != nil {
		return err
	}
	resp, err := authedClient(st).Invite(ctx, &proto.InviteRequest{Email: email, Role: *role})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Send this link to %s; it is valid for 7 days:\n", email)
	fmt.Println(resp.URL)
	return nil
}

func cmdDevices(ctx context.Context, fs *flag.FlagSet, args []string) error {
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	if err := st.requireLogin(); err != nil {
		return err
	}
	ds, err := authedClient(st).Devices(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPLATFORM\tLAST SEEN\tID")
	for _, d := range ds {
		name := d.Name
		if d.Current {
			name += " (this device)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, d.Platform, d.LastSeenAt.Local().Format("2006-01-02 15:04"), d.ID)
	}
	return w.Flush()
}

func cmdRevokeDevice(ctx context.Context, fs *flag.FlagSet, args []string) error {
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	st, err := loadState()
	if err != nil {
		return err
	}
	if err := st.requireLogin(); err != nil {
		return err
	}
	if err := authedClient(st).RevokeDevice(ctx, pos[0]); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Device signed out.")
	return nil
}
