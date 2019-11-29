# udptest

Test program to evaluate maximum udp send/receive performance for a golang server (DHCP in this case).

Borrows heavily from https://gist.github.com/jtblin/18df559cf14438223f93

Ideally you would run dhcperfcli instances on a machine separate from the test server.  A sample bash script to start multiple dhcperfcli instanaces is included along with the basic config file.

Usage:

You can either set the -concurrency parameter and the rx/tx/process worker count will default to that or you can set one or all of them individually to override -concurrency for that specific worker type.

./udptest -h\
Usage of ./udptest:\
  -addr string\
    	Listen Address (default ":8181")\
  -concurrency int\
    	Number of workers to run in parallel (default 24)\
  -cpuprofile string\
    	write cpu profile to file\
  -memprofile string\
    	write memory profile to file\
  -packets int\
    	Number of packets to send at once (default 100)\
  -process int\
    	Number of processing workers (-1 for goroutine per packet)\
  -rx int\
    	Number of receive workers\
  -tx int\
    	Number of transmit workers\
