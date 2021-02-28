import React, {FC} from "react";
import {useLocation} from "react-router-dom";
import {Navbar} from "@app/components/Navbar/Navbar";

export const NavbarContainer: FC = () => {
	const location = useLocation();

	console.log(location);

	return (
		<Navbar pathname={location.pathname}/>
	)
}