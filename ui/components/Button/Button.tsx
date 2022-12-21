import classNames from "classnames";
import React, { ButtonHTMLAttributes, DetailedHTMLProps } from "react";
import styles from "./Button.module.scss";

type ButtonProps = DetailedHTMLProps<
	ButtonHTMLAttributes<HTMLButtonElement>,
	HTMLButtonElement
>;

export default function Button(props: ButtonProps) {
	return (
		<button
			{...props}
			className={classNames(styles.libtn, props.className, {
				[styles.disabled]: props.disabled,
			})}
		>
			{props.children}
		</button>
	);
}
