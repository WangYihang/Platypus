import React from "react";
import qs from "qs";
import { Button, message } from "antd";

const axios = require("axios");

export default class CreateServerButton extends React.Component {
    render() {
        return <Button
            type="primary"
            onClick={() => {
                console.log(this.props);
                axios
                    .post(
                        [this.props.apiUrl, "/server"].join(""),
                        qs.stringify({
                            host: this.props.serverCreateHost,
                            port: this.props.serverCreatePort,
                            encrypted: this.props.serverCreateEncrypted,
                        })
                    )
                    .then((response) => {
                        if (response.data.status) {
                            message.success(
                                "Server created at: " +
                                response.data.msg.host +
                                ":" +
                                response.data.msg.port,
                                5
                            );
                            this.props.serverCreated(response.data.msg)
                        } else {
                            message.error(
                                "Server create failed: " + response.data.msg,
                                5
                            );
                        }
                    })
                    .catch((error) => {
                        message.error(
                            "Cannot connect to API EndPoint!" + error,
                            5
                        );
                    });
            }}
        >
            Add server
        </Button>
    }
}
