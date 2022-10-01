import axios from 'axios'
import React, { Component } from 'react'
import "./index.css"


export default class BeforeAuth extends Component {
    state = {
        username:"",
        password : "",
        captcha : "",
        tel:"",
        password2:"",
        flag:"login",
        randomNum:0,
    }

    changeImg=()=>{
        this.setState({randomNum:Math.random()})
    }

    getLogin=()=>{
        axios
            .get([this.props.baseUrl, "/login"].join(""))
            .then((response)=>{
                if (response.data.status) {
                    this.setState({flag:"login"})
                    this.changeImg()
                }else{
                    alert("获取登录页面失败")
                }
            })
            .catch(
                //
            )
    }

    getRegister=()=>{
        axios
            .get([this.props.baseUrl, "/register"].join(""))
            .then((response)=>{
                if (response.data.status) {
                    this.setState({flag:"register"})
                    this.changeImg()
                }else{
                    alert("获取注册页面失败")
                }
            })
            .catch(
                //
            )
    }

    getReset=()=>{
        axios
            .get([this.props.baseUrl, "/reset"].join(""))
            .then((response)=>{
                if (response.data.status) {
                    this.setState({flag:"reset"})
                    this.changeImg()
                }else{
                    alert("获取重置页面失败")
                }
            })
            .catch(
                //
            )


    }

    postData=()=>{
        if (this.state.flag === "login"){
            if (this.state.username !== ""){
                if (this.state.password !== ""){
                    if (this.state.captcha !== ""){
                        axios
                        .post([this.props.baseUrl, "/login"].join(""),{"username":this.state.username,"password":this.state.password,"captcha":this.state.captcha})
                        .then((response)=>{
                            if (response.data.status === false){
                                alert(response.data.msg)
                                this.changeImg()
                            }else{
                                //登录成功就可以渲染正常主页
                                this.props.refreshIndex()
                            }
                        })
                        .catch(
                            //
                        )
                    }else{
                        alert("验证码不能为空")
                    }
                }else{
                    alert("密码不能为空")
                }
            }else{
                alert("用户名不能为空")
            } 
        }
        if (this.state.flag === "register"){
            if (this.state.username !== ""){
                if (this.state.password !== ""){
                    if (this.state.password2 !== ""){
                        if (this.state.password === this.state.password2){
                            if (this.state.tel !== ""){
                                if (this.state.captcha !== ""){
                                    axios
                                    .post([this.props.baseUrl, "/register"].join(""),{"username":this.state.username,"password":this.state.password,"tel":this.state.tel,"captcha":this.state.captcha})
                                    .then((response)=>{
                                        if (response.data.status === false){
                                            alert(response.data.msg)
                                            this.changeImg()
                                        }else{
                                            //切换成登录页面
                                            this.setState({flag:"login"})
                                            this.changeImg()
                                        }
                                    })
                                    .catch(
                                        //
                                    )
                                }else{
                                    alert("验证码不能为空")
                                }
                            }else{
                                alert("手机号不能为空")
                            }
                        }else{
                            alert("两次密码不一致")
                        }
                    }else{
                        alert("确认密码不能为空")
                    }
                }else{
                    alert("密码不能为空")
                }
            }else{
                alert("用户名不能为空")
            }
        }
        if (this.state.flag === "reset"){
            if (this.state.username !== ""){
                if (this.state.password !== ""){
                    if(this.state.tel !== ""){
                        axios
                        .post([this.props.baseUrl, "/reset"].join(""),{"username":this.state.username,"password":this.state.password,"tel":this.state.tel})
                        .then((response)=>{
                            if (response.data.status){
                                this.setState({flag:"login"})
                                this.changeImg()
                            }else{
                                alert(response.data.msg)
                                this.changeImg()
                            }
                        })
                    }else{
                        alert("手机号不能为空")
                    }
                }else{
                    alert("新密码不能为空")
                }
            }else{
                alert("用户名不能为空")
            }
        }
    }

    getUsername=(e)=>{
        this.setState({username:e.target.value})
    }

    getPassword=(e)=>{
        this.setState({password:e.target.value})
    }

    getPassword2=(e)=>{
        this.setState({password2:e.target.value})
    }

    getCaptcha=(e)=>{
        this.setState({captcha:e.target.value})
    }

    getTel=(e)=>{
        this.setState({tel:e.target.value})
    }
  render() {
    return (
        <div className="box">
            <div className="btn-box">
                <div>
                    <button onClick={this.getLogin} >登录</button>
                    <button onClick={this.getRegister}>注册</button>
                    <button onClick={this.getReset}>重置</button>
                </div>
            </div>
            {this.state.flag === "login" ?
            <div>
                <h2>登录</h2>
                <div className="input-box">
                    <label>账号</label>
                    <input type="text" value={this.state.username} onChange={(e)=>{this.getUsername(e)}}/>
                </div>
                <div className="input-box">
                    <label>用户密码</label>
                    <input type="password" placeholder="不少于8位字符" value={this.state.password} onChange={(e)=>{this.getPassword(e)}}/>
                </div>
                <div className="input-box">
                    <label>验证码</label>
                    <input type="text" value={this.state.captcha} onChange={(e)=>{this.getCaptcha(e)}}/>
                    <img src={[this.props.baseUrl, "/captcha?v=", this.state.randomNum].join("")}></img>
                    <button onClick={this.changeImg}>点击刷新验证码</button>
                </div>
            </div> : this.state.flag === "register" ?
                    <div>
                        <h2>注册</h2>
                        <div className="input-box">
                            <label>账号</label>
                            <input type="text" value={this.state.username} onChange={(e) => {
                                this.getUsername(e)
                            }}/>
                        </div>
                        <div className="input-box">
                            <label>用户密码</label>
                            <input type="password" placeholder="不少于8位字符" value={this.state.password} onChange={(e) => {
                                this.getPassword(e)
                            }}/>
                        </div>
                        <div className="input-box">
                            <label>确认密码</label>
                            <input type="password" placeholder="不少于8位字符" value={this.state.password2} onChange={(e) => {
                                this.getPassword2(e)
                            }}/>
                        </div>
                        <div className="input-box">
                            <label>电话</label>
                            <input type="text" value={this.state.tel} onChange={(e) => {
                                this.getTel(e)
                            }}/>
                        </div>
                        <div className="input-box">
                            <label>验证码</label>
                            <input type="text" value={this.state.captcha} onChange={(e) => {
                                this.getCaptcha(e)
                            }}/>
                            <img src={[this.props.baseUrl, "/captcha?v=", this.state.randomNum].join("")}></img>
                            <button onClick={this.changeImg}>点击刷新验证码</button>
                        </div>
                    </div> :
                    <div>
                        <h2>重置</h2>
                        <div className="input-box">
                            <label>账号</label>
                            <input type="text" value={this.state.username} onChange={(e) => {
                                this.getUsername(e)
                            }}/>
                        </div>
                        <div className="input-box">
                            <label>新用户密码</label>
                            <input type="password" placeholder="不少于8位字符" value={this.state.password} onChange={(e) => {
                                this.getPassword(e)
                            }}/>
                        </div>
                        <div className="input-box">
                            <label>电话</label>
                            <input type="text" value={this.state.tel} onChange={(e) => {
                                this.getTel(e)
                            }}/>
                        </div>
                    </div>
            }
            <div className="btn-box">
                <div>
                    <button onClick={this.postData} >确定</button>
                </div>
            </div>
        </div>
    )
  }

}