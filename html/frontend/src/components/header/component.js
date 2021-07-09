import React from "react";
import { Layout } from "antd";

export default class PlatypusHeader extends React.Component {
    render() {
        return <Layout.Header className="header">
            <div className="logo" />
            <h1>
                <a href="https://github.com/WangYihang/Platypus" rel="noreferrer noopener">Platypus</a>
            </h1>
        </Layout.Header>
    }
}