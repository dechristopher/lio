import React, {FC, ReactElement} from "react";
import {
    BrowserRouter as Router,
    Switch,
    Route,
    Redirect
} from "react-router-dom";
import {NavbarContainer} from "@app/containers/Navbar/NavbarContainer";
import {PlayView} from "@app/views/Play/Play";
import {Modal} from "@components/Modal/Modal";
import {ModalContextProvider, useModalContext} from "@app/contexts/ModalContext";

export interface RootViewProps {
    /* optional prop to avoid empty interface */
    opt?: undefined;
}

export const RootViewContent: FC<RootViewProps> = () => {
    const [modalContext] = useModalContext();

    return (
        <Router>
            <NavbarContainer/>

            <Switch>
                <Route key={0} exact path="/" render={() => <Redirect to="/play"/>}/>
                <Route key={0} path="/play" render={() => <PlayView/>}/>
                <Route key={0} path="/learn" render={() => <h1>Learn View</h1>}/>
                <Route key={0} path="/watch" render={() => <h1>Watch View</h1>}/>
                <Route key={0} path="/players" render={() => <h1>Players View</h1>}/>

                <Route key={0} exact path="/u" render={() => <Redirect to="/u/username"/>}/>
                <Route key={0} path="/u/:username" render={() => <h1>User Profile View</h1>}/>
                <Route key={0} path="/account" render={() => <h1>Account Page</h1>}/>
            </Switch>

            <Modal
                isOpen={modalContext.isOpen}
                content={modalContext.content}
                hugContents
            />
        </Router>
    )
}

export const RootView = (): ReactElement => {
    return (
        <ModalContextProvider>
            <RootViewContent/>
        </ModalContextProvider>
    )
}
