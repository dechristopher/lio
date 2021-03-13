import React, {Children, cloneElement, FC, ReactNode} from 'react';
import {Button, ButtonProps} from "@components/Button/Button";

interface ButtonGroupProps {
    children: ReactNode | ReactNode[];
}

interface IButtonGroupProps extends FC<ButtonGroupProps> {
    Button: typeof Button;
}

/**
 * A collection of buttons with styling applied.
 *
 * @param {IButtonGroupProps} props - button group props
 *
 * @returns {Element} - button group component
 *
 * @example ButtonGroup
 * <ButtonGroup >
 *   <ButtonGroup.Button
 *      onClick={() => console.log("Clicked Button 1!")}
 *   >
 *      Button 1
 *   </ButtonGroup.Button>
 *   <ButtonGroup.Button
 *      onClick={() => console.log("Clicked Button 2!")}
 *   >
 *      Button 2
 *   </ButtonGroup.Button>
 *  </ButtonGroup>
 */
export const ButtonGroup: IButtonGroupProps = (props) => {
    // holds the modified versions of the button children
    let children: ReactNode | ReactNode[] = [];

    // dynamically modify the props of the buttons passed to the button group
    if (Array.isArray(props.children)) { // multiple button children
        children = Children.map(props.children, (child, childIdx) => {
            if (React.isValidElement<ButtonProps>(child)) {
                // set prop values on the children depending on what order they're in
                return cloneElement(
                    child,
                    {
                        roundLeftSide: childIdx === 0,
                        roundRightSide: childIdx === Children.count(props.children) - 1,
                        removeLeftMargin: childIdx > 0
                    },
                    child.props.children
                )
            }

            // we don't ever expect to return null because of the children restriction below
            return null;
        })
    } else { // one button child
        if (React.isValidElement<ButtonProps>(props.children)) {
            children = cloneElement(
                props.children,
                {
                    roundLeftSide: true,
                    roundRightSide: true
                },
                props.children
            )
        }
    }

    return (
        <span className="relative z-0 inline-flex shadow-sm rounded-md">
            {children}
        </span>
    )
}

ButtonGroup.propTypes = {
    children: (props, propName, componentName) => {
        const children = props[propName];

        let error = null;
        Children.forEach(children, (child) => {
            if (child.type !== Button) {
                error = new Error(
                    `Direct children of ${componentName} must be of type 'ButtonGroup.Button'`
                )
            }
        })

        return error;
    }
}

ButtonGroup.Button = Button;