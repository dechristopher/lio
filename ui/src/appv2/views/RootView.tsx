import React, {ReactElement} from "react";
import {BrowserRouter as Router, Redirect, Route, Switch} from "react-router-dom";
import {PlayView} from "@appv2/views/Play/Play";
import {Sidebar} from "@appv2/components/Sidebar/Sidebar";

export const RootViewContent = (): JSX.Element => {
    return (
        <Router>
            {/* Flexbox on sidebar + content, stretch container to fit full height */}
            <div className="flex h-screen">
                <Sidebar />

                <Switch>
                    <Route key={0} exact path="/" render={() => <Redirect to="/play"/>}/>
                    <Route key={2} path="/play" render={() => <PlayView/>}/>
                    <Route key={3} path="/learn" render={() => <h1>Learn View</h1>}/>
                    <Route key={4} path="/watch" render={() => <h1>Watch View</h1>}/>
                    <Route key={5} path="/players" render={() => <h1>Players View</h1>}/>

                    <Route key={6} exact path="/u" render={() => <Redirect to="/u/username"/>}/>
                    <Route key={7} path="/u/:username" render={() => <h1>User Profile View</h1>}/>
                    <Route key={8} path="/account" render={() => <h1>Account Page</h1>}/>
                </Switch>
            </div>
        </Router>
    )
}

export const RootView = (): ReactElement => {
    return (
        // todo in-case we want to use context
        // <ModalContextProvider>
        <RootViewContent/>
        // </ModalContextProvider>
    )
}