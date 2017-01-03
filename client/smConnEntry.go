package client

import (
	"io"
	"log"
	"net"
	"time"

	"gopkg.in/bunsim/miniss.v1"
	"gopkg.in/bunsim/natrium.v1"

	"github.com/bunsim/cluttershirt"
	"github.com/bunsim/geph/niaucchi2"
)

// smConnEntry is the ConnEntry state, where a connection to some entry node is established.
// => VerifyAccount if successful
// => ClearCache if unsuccessful
func (cmd *Command) smConnEntry() {
	log.Println("** => ConnEntry **")
	defer log.Println("** <= ConnEntry **")

	FANOUT := 1
	// TODO make this do something
	func(interface{}) {}(FANOUT)

	retline := make(chan *niaucchi2.Context)
	dedline := make(chan bool)
	for exit, entries := range cmd.entryCache {
		for _, xaxa := range entries {
			exit := exit
			xaxa := xaxa
			log.Println(xaxa.Addr, "from", exit)
			go func() {
				cand := niaucchi2.NewClientCtx()
				for i := 0; i < FANOUT; i++ {
					ctxid := make([]byte, 32)
					natrium.RandBytes(ctxid)
					rawconn, err := net.DialTimeout("tcp", xaxa.Addr, time.Second*10)
					if err != nil {
						log.Println("dial to", xaxa.Addr, err.Error())
						return
					}
					oconn, err := cluttershirt.Client(xaxa.Cookie, rawconn)
					if err != nil {
						log.Println("cluttershirt to", xaxa.Addr, err.Error())
						rawconn.Close()
						return
					}
					// 0x00 for a negotiable protocol
					oconn.Write([]byte{0x00})
					mconn, err := miniss.Handshake(oconn, cmd.identity)
					if err != nil {
						log.Println("miniss to", xaxa.Addr, err.Error())
						oconn.Close()
						return
					}
					if natrium.CTCompare(mconn.RemotePK(), xaxa.ExitKey.ToECDH()) != 0 {
						log.Println("miniss to", xaxa.Addr, "bad auth")
						oconn.Close()
						return
					}
					log.Println("miniss to", xaxa.Addr, "okay")
					// 0x02
					log.Println("gonna send", len(append([]byte{0x02}, ctxid...)))
					_, err = mconn.Write(append([]byte{0x02}, ctxid...))
					if err != nil {
						log.Println("ctxid to", xaxa.Addr, err.Error())
						mconn.Close()
						return
					}
					log.Println("id to", xaxa.Addr, "okay")
					err = cand.Absorb(mconn)
					if err != nil {
						log.Println("absorb to", xaxa.Addr, err.Error())
						mconn.Close()
						return
					}
				}
				select {
				case retline <- cand:
					cmd.stats.Lock()
					cmd.stats.netinfo.entry = natrium.HexEncode(
						natrium.SecureHash(xaxa.Cookie, nil)[:8])
					cmd.stats.netinfo.exit = exit
					cmd.stats.netinfo.prot = "cl-ni-2"
					cmd.stats.Unlock()
					log.Println(xaxa.Addr, "WINNER")
				case <-dedline:
					log.Println(xaxa.Addr, "failed race")
					cand.Tomb().Kill(io.ErrClosedPipe)
				}
			}()
		}
	}

	select {
	case <-time.After(time.Second * 10):
		log.Println("ConnEntry: failed to connect to anything within 10 seconds")
		cmd.smState = cmd.smClearCache
		return
	case ss := <-retline:
		close(dedline)
		cmd.currTunn = ss
		cmd.smState = cmd.smSteadyState
		return
	}
}

// smClearCache clears the cache and goes back to the entry point.
// => FindEntry always
func (cmd *Command) smClearCache() {
	log.Println("** => ClearCache **")
	defer log.Println("** <= ClearCache **")
	cmd.entryCache = nil
	cmd.exitCache = nil
	cmd.smState = cmd.smFindEntry
	if cmd.cdb != nil {
		cmd.cdb.Exec("DELETE FROM main WHERE k='bst.entries'")
	}
}
