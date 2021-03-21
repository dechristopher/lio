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
import {Game} from "@app/components/Game/Game";
import {DemoView} from "@app/views/Demo/Demo";

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
                <Route key={0} exact path="/" render={() => <Redirect to="/demo"/>}/>
                <Route key={1} path="/demo" render={() => <DemoView/>}/>
                <Route key={2} path="/play" render={() => <PlayView/>}/>
                <Route key={3} path="/learn" render={() => <Game />}/>
                <Route key={4} path="/watch" render={() => <h1>Watch View</h1>}/>
                <Route key={5} path="/players" render={() => <h1>Players View</h1>}/>

                <Route key={6} exact path="/u" render={() => <Redirect to="/u/username"/>}/>
                <Route key={7} path="/u/:username" render={() => <h1>User Profile View</h1>}/>
                <Route key={8} path="/account" render={() => <h1>Account Page</h1>}/>
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
