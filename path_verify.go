// Copyright © 2018 Immutability, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"regexp"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

func verifyPaths(b *PluginBackend) []*framework.Path {
	return []*framework.Path{
		&framework.Path{
			Pattern:      "verify",
			HelpSynopsis: "Verify that this claim (JWT) is good.",
			HelpDescription: `

Validate that this trustee made a claim.

`,
			Fields: map[string]*framework.FieldSchema{
				"token": &framework.FieldSchema{
					Type:        framework.TypeString,
					Description: "The JWT to verify.",
				},
			},
			ExistenceCheck: b.pathExistenceCheck,
			Callbacks: map[logical.Operation]framework.OperationFunc{
				logical.CreateOperation: b.pathVerifyClaim,
			},
		},
	}
}

func (b *PluginBackend) pathVerifyClaim(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	_, err := b.configured(ctx, req)
	if err != nil {
		return nil, err
	}
	rawToken := data.Get("token").(string)
	claims, _, err := b.verifyClaim(ctx, rawToken)
	if err == nil {
		return &logical.Response{
			Data: claims,
		}, nil
	}
	return nil, fmt.Errorf("Error verifying token")
}

func (b *PluginBackend) verifyClaim(ctx context.Context, rawToken string) (jwt.MapClaims, *ecdsa.PublicKey, error) {
	tokenWithoutWhitespace := regexp.MustCompile(`\s*$`).ReplaceAll([]byte(rawToken), []byte{})
	token := string(tokenWithoutWhitespace)

	jwtToken, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{})
	if err != nil || jwtToken == nil {
		return nil, nil, fmt.Errorf("cannot parse token")
	}
	unverifiedJwt := jwtToken.Claims.(jwt.MapClaims)
	if unverifiedJwt == nil {
		return nil, nil, fmt.Errorf("cannot get claims")
	}
	ethereumAddress := unverifiedJwt["iss"].(string)

	jti := unverifiedJwt["jti"].(string)
	signatureRaw := unverifiedJwt["eth"].(string)
	hash := hashKeccak256(jti)
	signature, err := hexutil.Decode(signatureRaw)

	if err != nil {
		return nil, nil, err
	}
	pubkey, err := crypto.SigToPub(hash, signature)

	if err != nil {
		return nil, nil, err
	}
	address := crypto.PubkeyToAddress(*pubkey)
	if strings.ToLower(ethereumAddress) == strings.ToLower(address.Hex()) {
		validateJwt, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
			return pubkey, nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf(err.Error())
		}
		claims := validateJwt.Claims.(jwt.MapClaims)
		err = claims.Valid()
		if err != nil {
			return nil, nil, err
		}
		return claims, pubkey, nil
	}
	return nil, nil, fmt.Errorf("Error verifying token")
}
