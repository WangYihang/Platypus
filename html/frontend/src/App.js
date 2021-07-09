import React from "react";
import "./App.css";

import Platypus from "./components/Platypus";

class App extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      bottom: "bottomLeft",
      serversMap: null,
      serversList: [],
      isModalVisible: false,
      currentServer: null,
      connectBack: "",
      serverCreateHost: "0.0.0.0",
      serverCreatePort: Math.floor(Math.random() * 65536),
    };
    this.handleCancel = this.handleCancel.bind(this);
    this.handleOk = this.handleOk.bind(this);
    this.selectServer = this.selectServer.bind(this);
    this.serverCreated = this.serverCreated.bind(this);
    this.setConnectBack = this.setConnectBack.bind(this);
    this.setCopied = this.setCopied.bind(this);
    this.setData = this.setData.bind(this);
    this.setServersMap = this.setServersMap.bind(this);
    this.showModal = this.showModal.bind(this);
  }

  showModal() {
    this.setState({
      isModalVisible: true,
    });
  };

  handleOk(hash) {
    this.setState({
      isModalVisible: false,
      connectBack: "",
    })
  };

  handleCancel() {
    this.setState({
      isModalVisible: false,
      connectBack: "",
    })
  };

  serverCreated(newServer) {
    this.setState({
      serversList: [...this.state.serversList, newServer],
    });
    const newServersMap = this.state.serversMap;
    newServersMap[newServer.hash] = newServer;
    this.setState({
      serversMap: newServersMap,
      serverCreatePort: Math.floor(Math.random() * 65536),
    });
  }

  selectServer(hash) {
    this.setState({
      currentServer: this.state.serversMap[hash],
    });
  }

  setConnectBack(value) {
    this.setState({ connectBack: value });
  }

  setCopied() {
    this.setState({ copied: true })
  }

  setData(data) {
    this.setState({
      serversMap: data.msg.servers,
      serversList: Object.values(data.msg.servers),
      currentServer: Object.values(data.msg.servers)[0],
      distributor: data.msg.distributor
    });
  }

  setServersMap(serversMap) {
    this.setState({
      serversMap: serversMap,
    });
  }

  render() {
    return (
        <Platypus
          bottom={this.state.bottom}
          connectBack={this.state.connectBack}
          currentServer={this.state.currentServer}
          distributor={this.state.distributor}
          isModalVisible={this.state.isModalVisible}
          selectServer={this.selectServer}
          serverCreatePort={this.state.serverCreatePort}
          serverCreated={this.serverCreated}
          serversList={this.state.serversList}
          serversMap={this.state.serversMap}
          setData={this.setData}
          setServersMap={this.setServersMap}
          showModal={this.showModal}
          handleCancel={this.handleCancel}
          handleOk={this.handleOk}
          setConnectBack={this.setConnectBack}
        />
    );
  }
}

export default App;