import React, {FC} from "react";
import classNames from "classnames";
import {Link} from "react-router-dom";

export interface DesktopNavLinkProps {
	to: string;
	active?: boolean;
}

export const DesktopNavLink: FC<DesktopNavLinkProps> = props => {
	return (
		<Link
			to={props.to}
			className={classNames("inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium", {
				"border-green-500 text-gray-900": props.active,
				"border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700": !props.active
			})}
		>
			{props.children}
		</Link>
	)
}