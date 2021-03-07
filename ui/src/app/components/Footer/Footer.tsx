import React, {FC} from "react";
import {Link} from "react-router-dom";

export const Footer: FC = () => {
	return (
		<div className="flex flex-wrap justify-evenly items-center">
			<Link className="whitespace-no-wrap px-2" to="/about">About</Link>
			<Link className="whitespace-no-wrap px-2" to="/faq">FAQ</Link>
			<Link className="whitespace-no-wrap px-2" to="/contact">Contact</Link>
			<Link className="whitespace-no-wrap px-2" to="/privacy">Privacy</Link>
			<a className="whitespace-no-wrap px-2" href="https://github.com/dechristopher/lio">Source Code</a>
			<a className="whitespace-no-wrap px-2" href="https://github.com/dechristopher/lio/issues">Report an Issue</a>
			<a className="whitespace-no-wrap px-2" href="https://github.com/dechristopher/lio/releases">Changelog</a>
		</div>
	)
}