import React, {FC, ReactNode} from "react";
import {Link} from "react-router-dom";

export namespace DesktopMenu {
	export interface LinkProps {
		type: "link";
		to: string;
	}

	export interface OptionProps {
		type: "option";
		onClick: () => void;
	}

	export interface ProfileProps {
		type: "profile";
		title: ReactNode;
		description: ReactNode;
	}
}

export const DesktopMenuOption: FC<DesktopMenu.LinkProps | DesktopMenu.OptionProps | DesktopMenu.ProfileProps> = props => {
	if (props.type === "option") {
		return (
			<span
				role="menuitem"
				onClick={props.onClick}
				className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
			>
				{props.children}
			</span>
		)
	} else if (props.type === "profile") {
		return (
			<div className="flex items-center px-4 py-2">
				<div>
					{props.title}
					{props.description}
				</div>
			</div>
		)
	} else {
		return (
			<Link
				to={props.to}
				role="menuitem"
				className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
			>
				{props.children}
			</Link>
		)
	}
}