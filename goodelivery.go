package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/adiabat/btcd/chaincfg"
	"github.com/howeyc/gopass"
)

/* goodelivery --
12kg of .9999 bitcoin

Offline bitcoin tools written in go.
Intended for low-velocity, high value, high security transactions.

commands:

[mnemonic]
	new /
	create a BIP39 mnemonic phrase

	adr /
	parse a BIP39 mnemonic phrase, returning addresses

	key /
	parse a BIP39 mnemonic phrase, returning WIF keys and addresses


(note, BIP38 not reccomended for new usage; use BIP39 compatible mne instead)
[BIP38]
	dec /
	decrypt BIP38 encrypted private key, returning WIF key

	enc /
	encrypt WIF private key into BIP38 format

[portxo]
	extract /
	extract a portable utxo from a serialized transaction

	insert /
	insert a WIF private key into a portable utxo

[signing]
	move /
	create signed trnasactions spending from one or more keyed portxos
	as this is an offline tool, the transaction is saved to disk and can be
	exported / printed

*/

func usage() {
	fmt.Printf("Usage:\n./goodelivery command -options\n")
	fmt.Printf("commands: new adr key dec enc extract insert move\n")
	//	fmt.Printf("or ./goodelivery BIP38 privkey\n")
}

// variables for a goodelivery session
type GDsession struct {
	command string

	inFileName *string // input filename, bypassing keyboard entry
	inFile     string  // content of loaded  input file

	outFileName *string // output filename, bypassing stdout

	// WIF can be supplied on the command line, or read from a file
	wifkey  *string // WIF string from command line
	wiffile *string // WIF filename

	// bip38 can be supplied from command line, or read from a file
	bip38key *string // bip38 string from command line

	destAdr *string // destination address to send to

	pass *string // password from cli args, bypassing entry (risky)

	bits  *int64 // bitlength of bip39 seed
	index *int64 // index for selecting txos from txs
	fee   *int64 // fee in satoshis per byte

	echo    *bool // echo input to screen? default false
	star    *bool // echo ****s to screen? default false
	verbose *bool // say more stuff. default false

	bip44   *bool // bip44 derivation paths (defaults to core's m/0'/0'/k')
	mainArg *bool // flag to set mainnet

	// defaults to testnet, not mainnet.  not reccommended for mainnet yet.
	NetParams *chaincfg.Params
}

// setFlags gets all the command line flags into the session struct.
// kindof annoying that it's all poiters though.
func (g *GDsession) setFlags(fset *flag.FlagSet) {
	g.inFileName = fset.String("in", "", "input file name")
	g.outFileName = fset.String("out", "", "output file name")

	// wiffile also doubles as password file for crack38
	g.wiffile = fset.String("wiffile", "", "file containging WIF private key")
	g.wifkey = fset.String("wif", "", "WIF private key")

	g.bip38key = fset.String("b38", "", "bip38 encrypted private key")

	g.destAdr = fset.String("dest", "", "destination bitcoin address")

	g.pass = fset.String("pass", "",
		"passphrase / salt given on command line (unsafe!)")

	g.bits = fset.Int64("b", 128, "bit length of mnemonic seed")
	g.index = fset.Int64("n", 21, "number (txo index, num of adrs)")

	g.fee = fset.Int64("fee", 21, "fee in satoshis per byte")

	g.echo = fset.Bool("echo", false, "echo text entry in the clear")
	g.star = fset.Bool("star", false, "echo text entry as ****")
	g.verbose = fset.Bool("v", false, "verbose mode")

	g.mainArg = fset.Bool("main", true, "use mainnet (not testnet3)")
	g.bip44 = fset.Bool("b44", false, "use bip44 key derivation (default m/0'/0'/k')")

}

func (g *GDsession) prompt(pr string) ([]byte, error) {
	//	if global cli arg pass is set, use that instead of prompting
	if *g.pass != "" {
		return []byte(*g.pass), nil
	}
	fmt.Printf(pr)
	// star gets priority; people might set echo and star on by accident
	if *g.star {
		return gopass.GetPasswdMasked()
	}
	if *g.echo {
		reader := bufio.NewReaderSize(os.Stdin, 32767)
		rawread, err := reader.ReadString('\n') // input finishes on enter key
		rawread = rawread[:len(rawread)-1]      // strip enter from end of read
		return []byte(rawread), err
	}
	return gopass.GetPasswd()
}

func (g *GDsession) output(s string) error {
	s += fmt.Sprintf("\n")
	if *g.outFileName != "" {
		return ioutil.WriteFile(*g.outFileName, []byte(s), 0600)
	}
	// no output file defined, so print to screen
	fmt.Printf(s)
	return nil
}

// returns the input file as a dehexlified byte slice
func (g *GDsession) inputHex() ([]byte, error) {
	// convert to bytes & return
	return hex.DecodeString(g.inFile)
}

// returns the input file as a trimmed string
func (g *GDsession) inputText() (string, error) {
	return g.inFile, nil
}

// LoadFiles reads in all files from disk and saves it to the session struct
func (g *GDsession) LoadFiles() error {

	// set network (can't do this in setFlags, needs to be after Parse() )
	if *g.mainArg {
		g.NetParams = &chaincfg.MainNetParams
	} else {
		g.NetParams = &chaincfg.TestNet3Params
	}

	if *g.wiffile != "" {
		// read from wif file
		b, err := ioutil.ReadFile(*g.wiffile)
		if err != nil {
			return err
		}
		// strip whitespace
		b = []byte(strings.TrimSpace(string(b)))
		// save into struct
		*g.wifkey = string(b)
	}

	if *g.inFileName != "" {
		b, err := ioutil.ReadFile(*g.inFileName)
		if err != nil {
			return err
		}
		// strip whitespace
		b = []byte(strings.TrimSpace(string(b)))
		// save into struct
		g.inFile = string(b)
	}

	return nil
}

func (g *GDsession) Run() error {
	var err error
	switch g.command {
	case "enc":
		err = g.enc38()
	case "dec":
		err = g.dec38()
	case "new":
		err = g.new39()
	case "adr":
		err = g.decode39(false)
	case "key":
		err = g.decode39(true)
	case "extract":
		err = g.extract()
	case "insert":
		err = g.insert()
	case "move":
		err = g.move()
	default:
		usage()
	}

	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func main() {

	if len(os.Args) < 2 {
		usage()
		return
	}

	fSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	var session GDsession
	session.setFlags(fSet)
	session.command = os.Args[1]

	fSet.Parse(os.Args[2:])

	err := session.LoadFiles()
	if err != nil {
		log.Fatal(err)
	}

	err = session.Run()
	if err != nil {
		log.Fatal(err)
	}
	return
}