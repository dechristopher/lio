import React, {FC} from "react";
import classNames from "classnames";
import {Link} from "react-router-dom";

export interface NavLinkProps {
	to: string;
	active?: boolean;
}

export const MobileNavLink: FC<NavLinkProps> = (props) => {
	return (
		<Link
			to={props.to}
			className={classNames("block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6", {
				"bg-green-50 border-green-500 text-green-700": props.active,
				"border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700": !props.active
			})}
		>
			{props.children}
		</Link>
	)
}