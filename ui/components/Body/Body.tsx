import React from "react";
import { FcWithChildren } from "../shared";
import { BodyStyle } from "./Body.css";

const Body: FcWithChildren = (props) => {
	return <div className={BodyStyle}>{props.children}</div>;
};

export default Body;
