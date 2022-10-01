import axios from 'axios'
import React, { Component } from 'react'
import LeftItem from '../LeftItem'

export default class LeftTable extends Component {
    state = {
        newRoleName:"",
    }

    getNewRoleName=(e)=>{
        this.setState({newRoleName:e.target.value})
    }

    postNewRoleName=()=>{
        axios
            .post([this.props.rbacUrl, "/role"].join(""),{"grade":this.state.newRoleName})
            .then((response)=>{
                if (response.data.status === false){
                    alert(response.data.msg)
                    return
                }
                this.props.getRoleAccesses()
                this.setState({newRoleName:""})
            })
            .catch((error) => {

                // message.error("Cannot connect to API EndPoint: " + error, 5);
            });


    }
    render() {
        const {leftList,getrightlist,getLeftName,hasCreate} = this.props
        return (
            <div className="leftTableDiv">
                <table className="table">
                    {
                        leftList.map(leftitem=>{
                            return <LeftItem rbacUrl={this.props.rbacUrl} leftitem={leftitem} getrightlist={getrightlist} getLeftName={getLeftName} hasCreate={hasCreate}/>
                        })
                    }
                    {hasCreate &&
                        <tr>
                            <td className="row">

                                <button onClick={this.postNewRoleName}>新建</button>
                                <input type="text" placeholder="创建新角色" value={this.state.newRoleName} onChange={this.getNewRoleName}/>
                            </td>
                        </tr>
                    }
                </table>
            </div>
        )
    }
}
