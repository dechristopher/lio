import React, {CSSProperties, FC, ReactNode} from 'react';
import classNames from "classnames";

interface InputProps {
    label?: ReactNode;
    centerLabel?: boolean;
    placeholder?: string;
    name?: string;
    id?: string;
    value?: string;
    onChange?: (value: string) => void;
    rightExtra?: ReactNode;
    style?: CSSProperties;
    inputClassName?: string;
}

export const Input: FC<InputProps> = (props) => {
    return (
        <div>
            {props.label ?
                <label
                    htmlFor={props.name}
                    className={classNames(
                        "block text-sm font-medium text-gray-700",
                        {
                            "w-max mx-auto": props.centerLabel,
                        }
                    )}>
                    {props.label}
                </label> : null}
            <div
                className="mt-1 flex">
                <input
                    value={props.value}
                    style={props.style}
                    type="text"
                    name={props.name}
                    id={props.id}
                    className={classNames(
                        "shadow-sm focus:ring-indigo-500 border focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md",
                        props.inputClassName
                    )}
                    placeholder={props.placeholder}
                    onChange={(e) => props.onChange?.(e.target.value)}
                />
                {props.rightExtra || null}
            </div>
        </div>
    );
};

Input.defaultProps = {
    placeholder: "",
    name: ""
}