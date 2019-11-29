package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/libp2p/go-reuseport"

	"golang.org/x/net/ipv4"
)

/* I added the following to control_unix.go in the github.com/libp2p/go-reuseport
   package in order to set the send/receive buffers on the socket.  Not sure they
   need to be 64Mb but the default didn't cut it.

                   err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF, 64*1024*1024)
                if err != nil {
                        return
                }

                err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, 64*1024*1024)
                if err != nil {
                        return
				}

Also set the following sysctls:
sudo sysctl -w net.core.rmem_max=68157440
sudo sysctl -w net.core.netdev_max_backlog=2000
*/

const (
	flushInterval = time.Duration(5) * time.Second
	maxQueueSize  = 200000
	uDPPacketSize = 1500
	packetBufSize = 1024 * 1024
)

var (
	address     string
	bufferPool  sync.Pool
	ops         uint64 = 0
	rops        uint64 = 0
	raops       uint64 = 0
	wops        uint64 = 0
	total       uint64 = 0
	rtotal      uint64 = 0
	ratotal     uint64 = 0
	wtotal      uint64 = 0
	flushTicker *time.Ticker
	nbWorkers   int
	nbProcess   int
	nbPackets   int
	nbRX        int
	nbTX        int
	cpuprofile  string
	memprofile  string
)

func init() {
	flag.StringVar(&address, "addr", ":8181", "Listen Address")
	flag.IntVar(&nbWorkers, "concurrency", runtime.NumCPU(), "Number of workers to run in parallel")
	flag.IntVar(&nbProcess, "process", 0, "Number of processing workers (-1 for goroutine per packet)")
	flag.IntVar(&nbRX, "rx", 0, "Number of receive workers")
	flag.IntVar(&nbTX, "tx", 0, "Number of transmit workers")
	flag.IntVar(&nbPackets, "packets", 100, "Number of packets to send at once")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flag.StringVar(&memprofile, "memprofile", "", "write memory profile to file")

}

type message struct {
	addr net.Addr
	msg  []byte
}

var recvQueue chan message
var sendQueue chan message

// Worker goroutine to process packets from the rx queue to tx queue
func process(c net.PacketConn) {
	for m := range recvQueue {
		handleMessage(c, m.addr, m.msg)
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	// Setup CPU profiling if requested
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Setup any overrides to number of workers
	if nbProcess == 0 {
		nbProcess = nbWorkers
	}
	if nbRX == 0 {
		nbRX = nbWorkers
	}
	if nbTX == 0 {
		nbTX = nbWorkers
	}

	log.Printf("Listening on %s with concurrency config:\n", address)
	log.Printf("    %d CPUs available\n", runtime.NumCPU())
	log.Printf("    %d receive workers\n", nbRX)
	if nbProcess == -1 {
		log.Printf("    process worker per packet\n")
	} else {
		log.Printf("    %d packet processing workers\n", nbProcess)
	}
	log.Printf("    %d send workers\n", nbTX)

	// Make queues for send and recieve
	recvQueue = make(chan message, maxQueueSize)
	sendQueue = make(chan message, maxQueueSize)

	// Open Socket with SO_REUSEADRR/SO_REUSEPORT and set TX/RX buffers
	conn, err := reuseport.ListenPacket("udp", address)
	if err != nil {
		log.Printf("Error opening socket: %v", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Setup number of workers as configured
	if nbProcess != -1 {
		for i := 0; i < nbProcess; i++ {
			go process(conn)
		}
	}
	for i := 0; i < nbRX; i++ {
		go receive(conn)
	}
	for i := 0; i < nbTX; i++ {
		go send(conn)
	}

	// Setup to handle program end and final statistics display
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			atomic.AddUint64(&rtotal, rops)
			atomic.AddUint64(&ratotal, raops)
			atomic.AddUint64(&total, ops)
			atomic.AddUint64(&wtotal, wops)
			log.Printf("Total rops %6d raops %d ops %d wops %d", rtotal, ratotal, total, wtotal)
			if cpuprofile != "" {
				pprof.StopCPUProfile()
			}
			if memprofile != "" {
				f, err := os.Create(memprofile)
				if err != nil {
					log.Fatal(err)
				}
				pprof.WriteHeapProfile(f)
				f.Close()
			}
			os.Exit(0)
		}
	}()

	// Setup period statistics display
	flushTicker = time.NewTicker(flushInterval)
	for range flushTicker.C {
		log.Printf("ROps/s %f RAOps/s %f POps/s %f WOps/s %f\n",
			float64(rops)/flushInterval.Seconds(),
			float64(raops)/flushInterval.Seconds(),
			float64(ops)/flushInterval.Seconds(),
			float64(wops)/flushInterval.Seconds())
		atomic.AddUint64(&rtotal, rops)
		atomic.AddUint64(&ratotal, raops)
		atomic.AddUint64(&total, ops)
		atomic.AddUint64(&wtotal, wops)
		atomic.StoreUint64(&rops, 0)
		atomic.StoreUint64(&raops, 0)
		atomic.StoreUint64(&ops, 0)
		atomic.StoreUint64(&wops, 0)
	}
}

// send transmits messages using WriteBatch from the sendQueue
func send(c net.PacketConn) {
	// get ipv4 Packet Connection so we can use batch functions
	conn := ipv4.NewPacketConn(c)

	// Create send buffer - this is probably not the best way
	ms := make([]ipv4.Message, nbPackets)
	for i := 0; i < len(ms); i++ {
		ms[i].Buffers = make([][]byte, 1)
	}

	// Collect up to nbPackets from the sendQueue or the 10ms timeout occurs
	// This would introduce a 10ms delay if packets are not arriving on the
	// queue fast enough.  10ms is an arbitrary number.
	idx, msgcount := 0, 0
	for {
		select {
		case msg := <-sendQueue:
			ms[idx].Buffers[0] = msg.msg
			// copy(ms[idx].Buffers[0], msg.msg)
			ms[idx].N = len(msg.msg)
			ms[idx].Addr = msg.addr
			if idx == nbPackets-1 {
				msgcount = nbPackets
				idx = 0
			} else {
				idx++
			}
		case <-time.After(10 * time.Millisecond):
			if idx > 0 {
				msgcount = idx
				idx = 0
			}
		}
		if msgcount == 0 {
			continue
		}
		n, err := conn.WriteBatch(ms[:msgcount], 0)
		if err != nil {
			log.Printf("send error: %v", err)
		}
		if n != msgcount {
			log.Printf("Write error: Had %d sent %d", msgcount, n)
		}
		// Update packets written counter
		atomic.AddUint64(&wops, uint64(n))
		msgcount = 0
	}

}

// receive packets from network and put on the receive queue or
// start a new goroutine to process each packet received
func receive(c net.PacketConn) {
	// get ipv4 Packet Conn
	conn := ipv4.NewPacketConn(c)

	// create receive buffer - again probably not best way
	ms := make([]ipv4.Message, 1024)
	for i := 0; i < len(ms); i++ {
		ms[i].Buffers = make([][]byte, 1)
		ms[i].Buffers[0] = make([]byte, uDPPacketSize)
	}

	// Receive packets using ReadBatch upto 1024 at a time
	for {
		n, err := conn.ReadBatch(ms, 0)
		if err != nil {
			log.Printf("ReadBatch error %s", err)
			continue
		}
		// log.Printf("ReadBatch got %d", n)
		for i := 0; i < n; i++ {
			// log.Printf("msg %d - %d %s", i, ms[i].N, ms[i].Addr.String())
			// fmt.Printf("%d %v\n", ms[i].N, ms[i].Buffers[0][0:ms[i].N-1])
			// fmt.Printf("address of buffer %p\n", ms[i].Buffers[0])
			// fmt.Printf("address of addr %p\n", ms[i].Addr)
			// Make a copy of packet from buffer...according to the docs I think
			// adding to the queue should be making a copy but it did not and let
			// to buffer corruption when they got reused.
			pkt := make([]byte, ms[i].N)
			_ = copy(pkt, ms[i].Buffers[0][0:ms[i].N-1])
			// Put message on receive queue or start a new gorouting depending on config
			if nbProcess == -1 {
				go handleMessage(c, ms[i].Addr, pkt)
			} else {
				recvQueue <- message{ms[i].Addr, pkt}
			}
		}
		atomic.AddUint64(&rops, 1)
		atomic.AddUint64(&raops, uint64(n))
	}
}

// bare minimum DHCP reply to request
func handleMessage(conn net.PacketConn, addr net.Addr, msg []byte) {
	// fmt.Printf("address of addr %p\n", addr)
	// fmt.Printf("address of msg %p\n", msg)

	// Create request from the raw bytes
	req, err := dhcpv4.FromBytes(msg)
	if err != nil {
		// log.Printf("Error parsing DHCPv4 request: %v\n", err)
		// log.Printf("%d %v\n", len(msg), msg)
		return
	}

	if req.OpCode != dhcpv4.OpcodeBootRequest {
		log.Printf("unsupported opcode %d. Only BootRequest (%d) is supported\n", req.OpCode, dhcpv4.OpcodeBootRequest)
		return
	}

	// Create reply from request
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		log.Printf("failed to build reply: %v\n", err)
		return
	}
	switch mt := req.MessageType(); mt {
	case dhcpv4.MessageTypeDiscover:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	case dhcpv4.MessageTypeRequest:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	default:
		log.Printf("plugins/server: Unhandled message type: %v\n", mt)
		return
	}

	// Fake other DHCP options
	resp.YourIPAddr = net.ParseIP("10.100.100.5")
	resp.UpdateOption(dhcpv4.OptServerIdentifier(net.ParseIP("10.200.200.119")))
	netmaskIP := net.ParseIP("255.255.255.0")
	netmaskIP = netmaskIP.To4()
	netmask := net.IPv4Mask(netmaskIP[0], netmaskIP[1], netmaskIP[2], netmaskIP[3])
	resp.Options.Update(dhcpv4.OptSubnetMask(netmask))
	DNSServer := net.ParseIP("4.4.4.4")
	var dnsServers4 []net.IP
	dnsServers4 = append(dnsServers4, DNSServer)
	if req.IsOptionRequested(dhcpv4.OptionDomainNameServer) {
		resp.Options.Update(dhcpv4.OptDNS(dnsServers4...))
	}

	// update packets process counter
	atomic.AddUint64(&ops, 1)

	// put message on the send queue
	sendQueue <- message{addr, resp.ToBytes()}
}
