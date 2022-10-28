import React from "react";
import { HyperLinkStyle } from "./HyperLink.css";

interface HyperLinkProps {
	link: string;
	text: string;
}

const HyperLink = (props: HyperLinkProps) => {
	return (
		<a href={props.link} className={HyperLinkStyle}>
			{props.text}
		</a>
	);
};

export default HyperLink;
