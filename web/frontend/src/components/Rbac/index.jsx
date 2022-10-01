import React, {Component} from 'react';
import SaveButton from '../SaveButton'
import LeftTable from '../LeftTable'
import RightTable from '../RightTable'
import axios from "axios";
import './index.css'

class Rbac extends Component {
    state = {
        leftList:[],
        rightList:[],
        leftName:"",
        hasCreate:false,
        flag:"",
    }
    getUserRoles=()=>{
        this.setState({hasCreate:false,flag:"0"})
        this.getLeftList(false)
        this.setState({rightList:[]})
    }

    getRoleAccesses=()=>{
        this.setState({hasCreate:true,flag:"1"})
        this.getLeftList(true)
        this.setState({rightList:[]})
    }

    getRightList=(rightlist)=>{
        this.setState({rightList:rightlist})
    }

    getLeftName=(leftName)=>{
        this.setState({leftName:leftName})
    }

    getLeftList=(flag)=>{
        if (flag){
            axios
                .get([this.props.rbacUrl, "/roles"].join(""))
                .then((response) => {
                    // this.state.leftList=response.data.msg
                    if (response.data.status){
                        this.setState({leftList:response.data.msg.rolenames})
                    }else{
                        alert(response.data.msg)
                    }


                })
                .catch((error) => {

                    // message.error("Cannot connect to API EndPoint: " + error, 5);
                });
        }
        else{
            axios
                .get([this.props.rbacUrl, "/users"].join(""))
                .then((response) => {
                    if (response.data.status){
                        this.setState({leftList:response.data.msg.usernames})
                    }else{
                        alert(response.data.msg)
                    }
                })
                .catch((error)=>{

                })
        }
    }
    render() {
        return (
            <section className="ant-layout" style={{padding: '0px 24px 24px'}}>
                <main className="ant-layout-content" style={{margin: '0px'}}>
                    <br/>
                    <table >
                        <SaveButton rbacUrl={this.props.rbacUrl} leftName={this.state.leftName} rightList={this.state.rightList} hasCreate={this.state.hasCreate} refreshFunc={this.state.hasCreate? this.getRoleAccesses: this.getUserRoles}></SaveButton>
                        <tr >
                            <td onClick={this.getUserRoles}>
                                <button >用户管理</button>
                            </td>
                            <td onClick={this.getRoleAccesses}>
                                <button >角色管理</button>
                            </td>
                        </tr>
                    </table>
                    <div className="holeTableDiv">
                        <LeftTable rbacUrl={this.props.rbacUrl} hasCreate={this.state.hasCreate} leftList={this.state.leftList} getrightlist={this.getRightList} getLeftName={this.getLeftName} getRoleAccesses={this.getRoleAccesses}></LeftTable>
                        <RightTable rightList={this.state.rightList} hasCreate={this.state.hasCreate}></RightTable>
                    </div>

                </main>
            </section>
        );
    }
}

export default Rbac;