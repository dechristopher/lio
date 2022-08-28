import React, {FC, ReactElement} from "react";
import {
    BrowserRouter,
    Switch,
    Route,
} from "react-router-dom";
import { Home } from "./Home/Home";

export const RootViewContent: FC = () => {
    return (
        <BrowserRouter>
            <Switch>
                <Route key={0} exact path="/" render={() => <Home />}/>
                <Route key={1} path="/game" render={() => "Game"}/>
            </Switch>
        </BrowserRouter>
    )
}

export const RootView = (): ReactElement => {
    return (
            <RootViewContent/>
    )
}
