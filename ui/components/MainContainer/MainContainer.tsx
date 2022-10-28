import React from "react";
import Body from "../Body/Body";
import { Footer } from "../Footer/Footer";
import { Header } from "../Header/Header";
import { FcWithChildren } from "../shared";

const MainContainer: FcWithChildren = (props) => {
	return (
		<div className="flex flex-col items-center pt-8">
			<Header />
			<Body>{props.children}</Body>
			<Footer />
		</div>
	);
};

export default MainContainer;
