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
                isOpen={modalContext.content !== undefined}
                content={modalContext.content}
                footerContent={
                    <>
                        <button type="button"
                                className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm">
                            Deactivate
                        </button>
                        <button type="button"
                                className="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm">
                            Cancel
                        </button>
                    </>
                }
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
