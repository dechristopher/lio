import React, {FC, ReactNode} from "react";
import classNames from "classnames";

export interface ButtonProps {
    children?: ReactNode | ReactNode[];
    removeLeftMargin?: boolean;
    roundLeftSide?: boolean;
    roundRightSide?: boolean;
    onClick?: () => void;
}

/**
 * Button component.
 *
 * @param {ButtonProps} props - button props
 * @param {ReactNode | ReactNode[]} props.children - the button's children
 * @param {boolean} props.removeLeftMargin - whether to apply -1 margin to the left side or not
 * @param {boolean} props.roundLeftSide - whether to round the left side or not
 * @param {boolean} props.roundRightSide - whether to round the right side or not
 * @param {() => void} props.onClick - button click handler
 *
 * @returns {Element} - button component
 *
 * @example Button
 * <Button
 *   removeLeftMargin: false,
 *   roundLeftSide: true,
 *   roundRightSide: true,
 *   onClick={() => console.log("You clicked me!")}
 * >
 *  I'm a button!
 * </Button>
 */
export const Button: FC<ButtonProps> = (props) => {
    return (
        <button
            type="button"
            onClick={props.onClick}
            className={classNames(
                "relative",
                "inline-flex",
                "items-center",
                "px-4",
                "py-2",
                "border",
                "border-gray-300",
                "bg-white",
                "text-sm",
                "font-medium",
                "text-gray-700",
                "hover:bg-gray-50",
                "focus:z-10",
                "focus:outline-none",
                "focus:ring-1",
                "focus:ring-indigo-500",
                "focus:border-indigo-500",
                {
                    "-ml-px": props.removeLeftMargin
                },
                {
                    "rounded-l-md": props.roundLeftSide
                },
                {
                    "rounded-r-md": props.roundRightSide
                }
            )}>
            {props.children}
        </button>
    )
}