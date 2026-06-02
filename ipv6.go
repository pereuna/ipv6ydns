package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
	"flag"
)

const host = "jedi.ydns.eu"

func main() {

	var verbose bool

	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	currentIPs, err := localGlobalIPv6()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dnsIPs, err := resolveAAAA(host)
	if err != nil {
		fmt.Fprintln(os.Stderr, "DNS AAAA haku epäonnistui:", err)
		os.Exit(1)
	}
	if verbose {
		fmt.Println("Paikalliset global IPv6 -osoitteet:")
		for _, ip := range currentIPs {
			fmt.Println(" ", ip)
		}

		fmt.Println()
		fmt.Println(host, "AAAA:")
		for _, ip := range dnsIPs {
			fmt.Println(" ", ip)
		}

	}

	match := false
	for _, localIP := range currentIPs {
		for _, dnsIP := range dnsIPs {
			if localIP.Equal(dnsIP) {
				match = true
			}
		}
	}

	//fmt.Println()

	if match {
		if verbose {
			fmt.Println("OK: DNS AAAA vastaa tämän koneen IPv6-osoitetta. Ei päivitetä.")
		}
		return
	}

	currentIP := currentIPs[0]

	fmt.Println("DNS AAAA ei vastaa paikallista IPv6-osoitetta.")
	fmt.Println("Päivitetään YDNS:")
	fmt.Println(" host:", host)
	fmt.Println(" ip:  ", currentIP)

	if err := updateYDNS(host, currentIP.String()); err != nil {
		fmt.Fprintln(os.Stderr, "YDNS-päivitys epäonnistui:", err)
		os.Exit(2)
	}

	fmt.Println("YDNS-päivityspyyntö tehty.")
}

func updateYDNS(hostname, ip string) error {
	user := os.Getenv("YDNS_USER")
	pass := os.Getenv("YDNS_PASSWD")

	if user == "" {
		return fmt.Errorf("YDNS_USER puuttuu ympäristöstä")
	}
	if pass == "" {
		return fmt.Errorf("YDNS_PASSWD puuttuu ympäristöstä")
	}

	u, err := url.Parse("https://ydns.io/api/v1/update/")
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("host", hostname)
	q.Set("ip", ip)
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}

	req.SetBasicAuth(user, pass)
	req.Header.Set("User-Agent", "openbsd-rpi4-ydns-ipv6-updater/1.0")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	fmt.Println("YDNS HTTP status:", resp.Status)
	if len(body) > 0 {
		fmt.Println("YDNS vastaus:", string(body))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("YDNS palautti virhekoodin %s", resp.Status)
	}

	return nil
}

func resolveAAAA(name string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip6", name)
	if err != nil {
		return nil, err
	}

	var out []net.IP

	for _, ip := range addrs {
		if ip.To4() == nil && ip.To16() != nil {
			out = append(out, ip)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("nimellä %s ei ole AAAA-tietuetta", name)
	}

	return out, nil
}

func localGlobalIPv6() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("Interfaces epäonnistui: %w", err)
	}

	var out []net.IP

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if isUsableGlobalIPv6(ip) {
				out = append(out, ip)
			}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("ei löytynyt paikallista global IPv6 -osoitetta")
	}

	return out, nil
}

func ipFromAddr(addr net.Addr) net.IP {
	switch a := addr.(type) {
	case *net.IPNet:
		return a.IP
	case *net.IPAddr:
		return a.IP
	default:
		return nil
	}
}

func isUsableGlobalIPv6(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if ip.To4() != nil {
		return false
	}

	ip = ip.To16()
	if ip == nil {
		return false
	}

	if !ip.IsGlobalUnicast() {
		return false
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}

	return true
}
