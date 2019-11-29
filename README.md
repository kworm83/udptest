# udptest

Test program to evaluate maximum udp send/receive performance for a golang server (DHCP in this case).

Borrows heavily from https://gist.github.com/jtblin/18df559cf14438223f93

Ideally you would run dhcperfcli instances on a machine separate from the test server.  A sample bash script to start multiple dhcperfcli instanaces is included along with the basic config file.