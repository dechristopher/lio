import React, {FC, MutableRefObject} from "react";
import {NavbarProps} from "@app/components/Navbar/Navbar";
import classNames from "classnames";

export interface MobileNavbarMenuButtonProps extends NavbarProps {
	menuOpen: boolean;
	menuButtonRef: MutableRefObject<null>;
	setMenuOpen: React.Dispatch<React.SetStateAction<boolean>>;
}

export const MobileNavbarMenuButton: FC<MobileNavbarMenuButtonProps> = props => {
	return (
		<div className="-ml-2 mr-2 flex items-center md:hidden">
			{/* Mobile menu button */}
			<button
				ref={props?.menuButtonRef}
				type="button"
				aria-expanded="false"
				aria-controls="mobile-menu"
				onClick={() => props?.setMenuOpen(open => !open)}
				className="inline-flex items-center justify-center p-2 rounded-md text-gray-400 hover:text-gray-500 hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-green-500"
			>
				<span className="sr-only">Open main menu</span>
				{/* Icon when menu is closed. */}
				{/*
									Heroicon name: outline/menu

									Menu open: "hidden", Menu closed: "block"
								*/}
				<svg
					className={classNames("h-6 w-6", {
						"hidden": props?.menuOpen,
						"block": !props?.menuOpen,
					})}
					xmlns="http://www.w3.org/2000/svg"
					fill="none"
					viewBox="0 0 24 24"
					stroke="currentColor"
					aria-hidden="true"
				>
					<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16M4 18h16" />
				</svg>
				{/* Icon when menu is open. */}
				{/*
									Heroicon name: outline/x

									Menu open: "block", Menu closed: "hidden"
								*/}
				<svg className={classNames("h-6 w-6", {
					"hidden": !props?.menuOpen,
					"block": props?.menuOpen,
				})}  xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
					<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>
	)
}