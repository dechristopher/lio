import React, {FC} from "react";
// import {
// 	BrowserRouter as Router,
// 	Switch,
// 	Route,
// 	Redirect
// } from "react-router-dom";
import {Navbar} from "@app/components/Navbar";

export interface RootViewProps {
	/* optional prop to avoid empty interface */
	opt?: undefined;
}

export const RootView: FC<RootViewProps> = () => {
	return (
		// <Router>
			<Navbar />
		// 	<Switch>
		// 		<Route key={0} exact path="/" render={() => <Redirect to="/play" />} />
		// 		<Route key={0} path="/play" render={() => <h1>Play View</h1>} />
		// 		<Route key={0} path="/learn" render={() => <h1>Learn View</h1>} />
		// 		<Route key={0} path="/watch" render={() => <h1>Watch View</h1>} />
		// 		<Route key={0} path="/players" render={() => <h1>Players View</h1>} />
		//
		// 		<Route key={0} exact path="/profile/me" render={() => <h1>Your Profile View</h1>} />
		// 		<Route key={0} path="/profile/:username" render={() => <h1>User Profile View</h1>} />
		// 	</Switch>
		// </Router>
	)
}