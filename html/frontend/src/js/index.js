var app = new Vue({
    el: '#app',
    data: {
        servers: [],
        clientsTableData: [],
    },
    mounted() {
        axios
            .get('http://127.0.0.1:7331/server')
            .then(response => (
                this.servers = response.data.msg
            ))
            .then(function() {
                if (Object.keys(app.servers) == 0) {
                    this.clientsTableData = []
                } else {
                    axios
                        .get('http://127.0.0.1:7331/server/' + Object.keys(app.servers)[0] + '/client')
                        .then(response => (
                            app.clientsTableData = Object.values(response.data.msg)
                        ))
                }
            })
    },

    computed: {
        serverDescription: function() {
            return function(server) {
                return server.host + ":" + server.port + "(" + Object.keys(server.clients).length + ")"
            };
        }
    }
})

// var term = new Terminal();
// term.open(document.getElementById('terminal'));
// // term.write('Hello from \x1B[1;3;31mxterm.js\x1B[0m $ ')