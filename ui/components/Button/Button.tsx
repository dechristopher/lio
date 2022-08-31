import React, { ButtonHTMLAttributes, DetailedHTMLProps } from "react";
import { BaseButtonStyle } from "./Button.css";

type ButtonProps = DetailedHTMLProps<
	ButtonHTMLAttributes<HTMLButtonElement>,
	HTMLButtonElement
>;

export default function Button(props: ButtonProps) {
	return (
		<button {...props} className={`${props.className ?? BaseButtonStyle}`}>
			{props.children}
		</button>
	);
}
