import React, {FC, ReactNode} from "react"
import classNames from "classnames";

export interface CardProps {
	header?: ReactNode;
	children?: ReactNode | ReactNode[];
	footer?: ReactNode;
	noPad?: boolean;
}

/**
 * @description Card - Shared component implementing the TailwindUI Card spec
 * @see https://tailwindui.com/components/application-ui/layout/panels
 *
 * @param {CardProps} props - The full prop spec for the card component
 * @param {ReactNode} props.header - The optional react node placed at the top of the card
 * @param {ReactNode | ReactNode[]} props.children - The optional react node(s) placed
 * within the container of the card
 * @param {ReactNode} props.footer - The optional react node placed at the bottom of the card
 * @param {boolean} props.noPad - The optional boolean to omit card padding if supplied by the
 * content's margin or other source.
 *
 * @returns {JSX.Element} Card - The formatted Card JSX.Element
 */
export const Card: FC<CardProps> = props => {
	return (
		<div className="bg-white overflow-hidden shadow rounded-lg divide-y divide-gray-200">

			{/* Begin header content */}
			{props?.header ? (
				<div className={classNames({
					"px-4 py-5 sm:px-6": !props.noPad
				})}>
					{props.header}
				</div>
			) : null}
			{/* End header content */}

			<div className={classNames({
				"px-4 py-5 sm:p-6": !props.noPad
			})}>
				{props.children}
			</div>

			{/* Begin footer content */}
			{props?.footer ? (
				<div className={classNames({
					"px-4 py-5 sm:px-6": !props.noPad
				})}>
					{props.footer}
				</div>
			) : null}
			{/* End footer content */}

		</div>
	)
}