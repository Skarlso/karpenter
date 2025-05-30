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
	"sync"

	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
	karpoptions "sigs.k8s.io/karpenter/pkg/operator/options"

	"github.com/aws/karpenter-provider-aws/kwok/cloudprovider"
	"github.com/aws/karpenter-provider-aws/kwok/operator"
	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/pkg/controllers"
)

func main() {
	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	kwokAWSCloudProvider := cloudprovider.New(
		op.InstanceTypesProvider,
		op.InstanceProvider,
		op.EventRecorder,
		op.GetClient(),
		op.AMIProvider,
		op.SecurityGroupProvider,
		op.CapacityReservationProvider,
	)
	cloudProvider := metrics.Decorate(kwokAWSCloudProvider)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)

	if karpoptions.FromContext(ctx).FeatureGates.ReservedCapacity {
		v1.CapacityReservationsEnabled = true
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-op.Elected()
		op.EC2API.StartBackupThread(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-op.Elected()
		op.EC2API.StartKillNodeThread(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-op.Elected()
		op.EC2API.ReadBackup(ctx)
	}()

	op.
		WithControllers(ctx, corecontrollers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			clusterState,
		)...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.Manager,
			op.Config,
			op.Clock,
			op.EC2API,
			op.GetClient(),
			op.EventRecorder,
			op.UnavailableOfferingsCache,
			op.SSMCache,
			op.ValidationCache,
			cloudProvider,
			op.SubnetProvider,
			op.SecurityGroupProvider,
			op.InstanceProfileProvider,
			op.InstanceProvider,
			op.PricingProvider,
			op.AMIProvider,
			op.LaunchTemplateProvider,
			op.VersionProvider,
			op.InstanceTypesProvider,
			op.CapacityReservationProvider,
			op.AMIResolver,
		)...).
		Start(ctx)
	wg.Wait()
}
