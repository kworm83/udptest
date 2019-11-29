#!/bin/bash
TESTTIME=30

echo "*****************" > logs/perf1.log
echo "#1 10.200.200.120" >> logs/perf1.log
echo "*****************" >> logs/perf1.log
/opt/freeradius/4.0.x/bin/dhcperfcli -T -s 300 -L $TESTTIME -p 64 -f dhcp.txt -g 10.200.200.120 10.200.200.119 dora >> logs/perf1.log &

echo "*****************" > logs/perf2.log
echo "#2 10.200.200.250" >> logs/perf2.log
echo "*****************" >> logs/perf2.log
/opt/freeradius/4.0.x/bin/dhcperfcli -T -s 300 -L $TESTTIME -p 64 -f dhcp.txt -g 10.200.200.250 10.200.200.119 dora >> logs/perf2.log &

echo "*****************" > logs/perf3.log
echo "#3 10.200.200.251" >> logs/perf3.log
echo "*****************" >> logs/perf3.log
/opt/freeradius/4.0.x/bin/dhcperfcli -T -s 300 -L $TESTTIME -p 64 -f dhcp.txt -g 10.200.200.251 10.200.200.119 dora >> logs/perf3.log &

echo "*****************" > logs/perf4.log
echo "#4 10.200.200.252" >> logs/perf4.log
echo "*****************" >> logs/perf4.log
/opt/freeradius/4.0.x/bin/dhcperfcli -T -s 300 -L $TESTTIME -p 64 -f dhcp.txt -g 10.200.200.252 10.200.200.119 dora >> logs/perf4.log &

echo "*****************" > logs/perf5.log
echo "#5 10.200.200.253" >> logs/perf5.log
echo "*****************" >> logs/perf5.log
/opt/freeradius/4.0.x/bin/dhcperfcli -T -s 300 -L $TESTTIME -p 64 -f dhcp.txt -g 10.200.200.253 10.200.200.119 dora >> logs/perf5.log &
